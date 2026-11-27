package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCheckSocketDir(t *testing.T) {
	euid, gid := os.Geteuid(), os.Getegid()
	cases := []struct {
		mode      os.FileMode
		clientGID int
		ok        bool
	}{
		{0700, -1, true},
		{0710, -1, false},
		{0750, -1, false},
		{0710, gid, true},
		{0700, gid, false},
		{0750, gid, false},
		{0770, gid, false},
		{0710, gid + 1, false}, // the directory belongs to another group
	}
	for _, c := range cases {
		dir := t.TempDir()
		if err := os.Chmod(dir, c.mode); err != nil {
			t.Fatal(err)
		}
		info, err := os.Lstat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := checkSocketDir(info, euid, c.clientGID); (err == nil) != c.ok {
			t.Errorf("mode %o, client group %d: error %v, want ok=%v", c.mode, c.clientGID, err, c.ok)
		}
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Lstat(dir)
	if checkSocketDir(info, euid+1, -1) == nil {
		t.Error("accepted a directory owned by another user")
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0700); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Lstat(file)
	if checkSocketDir(info, euid, -1) == nil {
		t.Error("accepted a file")
	}
}

func TestLookupGroup(t *testing.T) {
	if gid, err := lookupGroup(""); err != nil || gid != -1 {
		t.Fatalf("empty group = %d, %v", gid, err)
	}
	if _, err := lookupGroup("0"); err == nil {
		t.Fatal("accepted root's group")
	}
	if _, err := lookupGroup("no-such-group-ember"); err == nil {
		t.Fatal("accepted an unknown group")
	}
	own := os.Getegid()
	if own == 0 {
		t.Skip("running with root's group")
	}
	if gid, err := lookupGroup(strconv.Itoa(own)); err != nil || gid != own {
		t.Fatalf("numeric group = %d, %v", gid, err)
	}
}

// The socket is shared with the client group only when one is given.
func TestServeSocketPermissions(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the router refuses to run as root")
	}
	for _, c := range []struct {
		clientGID int
		dirMode   os.FileMode
		want      os.FileMode
	}{{-1, 0700, 0600}, {os.Getegid(), 0710, 0660}} {
		root := t.TempDir()
		config := filepath.Join(root, "policy.json")
		policy := `{"probe":{"profile":"trusted","image":"sha256:` + strings.Repeat("0", 64) + `"}}`
		if err := os.WriteFile(config, []byte(policy), 0600); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, "run")
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, c.dirMode); err != nil {
			t.Fatal(err)
		}
		socket := filepath.Join(dir, "router.sock")
		errc := make(chan error, 1)
		go func() { errc <- serve(config, socket, c.clientGID) }()

		client := http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		}}}
		// An unknown agent is refused before any container runtime is used.
		var resp *http.Response
		var err error
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			select {
			case err := <-errc:
				t.Fatalf("serve stopped early: %v", err)
			default:
			}
			if resp, err = client.Post("http://router/v1/runs", "application/json",
				strings.NewReader(`{"agent":"missing","input":"hi"}`)); err == nil {
				break
			}
		}
		if err != nil {
			t.Fatalf("router did not answer: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("unknown agent status %d", resp.StatusCode)
		}
		info, err := os.Stat(socket)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != c.want {
			t.Errorf("client group %d: socket mode %o, want %o", c.clientGID, info.Mode().Perm(), c.want)
		}
		if gid := info.Sys().(*syscall.Stat_t).Gid; c.clientGID >= 0 && gid != uint32(c.clientGID) {
			t.Errorf("socket group %d, want %d", gid, c.clientGID)
		}
		// serve installs its signal handler before it listens.
		syscall.Kill(os.Getpid(), syscall.SIGTERM)
		select {
		case err := <-errc:
			if err != nil {
				t.Fatalf("serve: %v", err)
			}
		case <-time.After(20 * time.Second):
			t.Fatal("serve did not stop")
		}
	}
}
