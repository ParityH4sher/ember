package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"
)

type runtimeLauncher struct{ clientDir string }

// Engine and runtime selection are never supplied by the task.
func (l runtimeLauncher) command(ctx context.Context, profile string, args ...string) *exec.Cmd {
	if profile == "isolated" {
		return exec.CommandContext(ctx, "docker", append([]string{"--host=unix:///var/run/docker.sock", "--config", l.clientDir}, args...)...)
	}
	return exec.CommandContext(ctx, "podman", append([]string{"--remote=false"}, args...)...)
}

func runArgs(a agent, id string) []string {
	args := []string{"run", "--name", id, "--label=dev.ember.sandbox-router=v1", "--rm", "--interactive",
		"--pull=never", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges",
		"--user=65532:65532", "--workdir=/work", "--tmpfs=/work:rw,nosuid,nodev,size=32m,mode=1777",
		"--tmpfs=/tmp:rw,nosuid,nodev,noexec,size=16m,mode=1777", "--env=HOME=/work",
		"--cpus=1", "--memory=512m", "--memory-swap=512m", "--pids-limit=128",
		"--log-driver=none", "--no-healthcheck", "--entrypoint=/ember-agent"}
	if a.Profile == "isolated" {
		args = append(args, "--runtime=io.containerd.kata.v2")
	} else {
		args = append(args, "--http-proxy=false", "--image-volume=ignore", "--read-only-tmpfs=false", "--timeout=60")
	}
	return append(args, a.Image)
}

// Abort at the output boundary instead of buffering or logging an unbounded stream.
type cappedOutput struct {
	buf      bytes.Buffer
	overflow bool
	cancel   context.CancelFunc
}

func (b *cappedOutput) Write(p []byte) (int, error) {
	if len(p) > (64<<10)-b.buf.Len() {
		b.overflow = true
		b.cancel()
		return 0, errors.New("agent output too large")
	}
	return b.buf.Write(p)
}

func (l runtimeLauncher) run(parent context.Context, a agent, id, input string) (string, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	switch a.Profile {
	case "trusted":
		info, err := l.command(ctx, a.Profile, "info", "--format", "{{.Host.Security.Rootless}} {{.Host.CgroupsVersion}}").Output()
		if err != nil || strings.TrimSpace(string(info)) != "true v2" {
			return "", errors.New("trusted profile requires local rootless Podman and cgroup v2")
		}
	case "isolated":
		// Docker would otherwise create writable anonymous volumes from VOLUME.
		volumes, err := l.command(ctx, a.Profile, "image", "inspect", "--format", "{{len .Config.Volumes}}", a.Image).Output()
		if err != nil || strings.TrimSpace(string(volumes)) != "0" {
			return "", errors.New("isolated profile requires a local Docker image without declared volumes")
		}
	default:
		return "", errors.New("unknown isolation profile")
	}
	cmd := l.command(ctx, a.Profile, runArgs(a, id)...)
	out := cappedOutput{cancel: cancel}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = strings.NewReader(input), &out, io.Discard
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	if err != nil {
		// Killing the client alone does not necessarily stop a daemon-side job.
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if rmErr := l.command(cleanup, a.Profile, "rm", "-f", id).Run(); rmErr != nil {
			log.Printf("run=%s cleanup not confirmed (container may already be removed): %v", id, rmErr)
		}
		if out.overflow {
			return "", errors.New("agent exceeded 64 KiB output limit")
		}
		if parent.Err() != nil {
			return "", parent.Err()
		}
		return "", fmt.Errorf("%s launch or agent failed: %w", a.Profile, err)
	}
	return out.buf.String(), nil
}
