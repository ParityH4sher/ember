package projects

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func chatVault(t *testing.T) (*Vaults, string) {
	t.Helper()
	root := t.TempDir()
	v, err := OpenVaults(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Create(Project{ID: "desk", Name: "Desk"}); err != nil {
		t.Fatal(err)
	}
	return v, root
}

func newChat(updated time.Time) Chat {
	return Chat{Schema: ChatSchema, ID: NewID(), Agent: "chat-fixture", Created: updated, Updated: updated, Messages: []ChatMessage{}}
}

func TestChatRoundTripAndOrder(t *testing.T) {
	v, root := chatVault(t)
	if chats, err := v.Chats("desk"); err != nil || chats == nil || len(chats) != 0 {
		t.Fatalf("Chats before any chat = %v, %v; want an empty list", chats, err)
	}
	base := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	older, newer := newChat(base), newChat(base.Add(time.Minute))
	for _, c := range []Chat{older, newer} {
		if err := v.CreateChat("desk", c); err != nil {
			t.Fatal(err)
		}
	}
	if err := v.CreateChat("desk", older); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("duplicate ID: %v", err)
	}
	chats, err := v.Chats("desk")
	if err != nil || len(chats) != 2 || chats[0].ID != newer.ID || chats[1].ID != older.ID {
		t.Fatalf("Chats = %+v, %v; want newest first", chats, err)
	}

	updated, err := v.UpdateChat("desk", older.ID, 0, func(c *Chat) error {
		c.Title = "Filament <choice> & cost"
		c.Updated = base.Add(2 * time.Minute)
		c.Messages = append(c.Messages,
			ChatMessage{ID: NewID(), Role: "user", Content: "Which filament?", Created: base},
			ChatMessage{ID: NewID(), Role: "agent", Status: StatusPending, Created: base})
		return nil
	})
	if err != nil || len(updated.Messages) != 2 {
		t.Fatal(updated, err)
	}
	got, err := v.Chat("desk", older.ID)
	if err != nil || got.Title != "Filament <choice> & cost" || got.Messages[1].Status != StatusPending {
		t.Fatalf("Chat = %+v, %v", got, err)
	}
	if chats, _ := v.Chats("desk"); chats[0].ID != older.ID {
		t.Fatal("update did not move the chat to the top")
	}

	// The file is readable, indented JSON in the project, not escaped HTML.
	data, err := os.ReadFile(filepath.Join(root, "desk", "chats", older.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var file map[string]any
	if err := json.Unmarshal(data, &file); err != nil || file["schema"] != ChatSchema {
		t.Fatalf("stored file: %v %v", file["schema"], err)
	}
	if !strings.Contains(string(data), "\n  \"title\": \"Filament <choice> & cost\"") {
		t.Fatalf("unexpected encoding:\n%s", data)
	}

	// The transcript survives an API restart.
	reopened, err := OpenVaults(root)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := reopened.Chat("desk", older.ID); err != nil || len(again.Messages) != 2 {
		t.Fatalf("after restart: %+v, %v", again, err)
	}
}

func TestChatUpdateRules(t *testing.T) {
	v, _ := chatVault(t)
	c := newChat(time.Now().UTC())
	if err := v.CreateChat("desk", c); err != nil {
		t.Fatal(err)
	}
	refused := errors.New("refused")
	if _, err := v.UpdateChat("desk", c.ID, 0, func(c *Chat) error { c.Title = "changed"; return refused }); err != refused {
		t.Fatalf("change error: %v", err)
	}
	if got, _ := v.Chat("desk", c.ID); got.Title != "" {
		t.Fatal("saved a refused change")
	}
	if _, err := v.UpdateChat("desk", c.ID, 0, func(c *Chat) error {
		c.Messages = append(c.Messages, ChatMessage{ID: NewID(), Role: "system", Content: "x"})
		return nil
	}); !errors.Is(err, ErrChatInvalid) {
		t.Fatalf("invalid role: %v", err)
	}
	if _, err := v.UpdateChat("desk", c.ID, MaxChatBytes, func(*Chat) error { return nil }); !errors.Is(err, ErrChatFull) {
		t.Fatalf("reserve beyond the limit: %v", err)
	}
	big := strings.Repeat("x", 64<<10)
	var err error
	for i := 0; i < 20 && err == nil; i++ {
		_, err = v.UpdateChat("desk", c.ID, 0, func(c *Chat) error {
			c.Messages = append(c.Messages, ChatMessage{ID: NewID(), Role: "user", Content: big})
			return nil
		})
	}
	if !errors.Is(err, ErrChatFull) {
		t.Fatalf("growing past 1 MiB: %v", err)
	}
}

func TestChatLookupErrors(t *testing.T) {
	v, root := chatVault(t)
	if _, err := v.Chats("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown project: %v", err)
	}
	if err := v.CreateChat("missing", newChat(time.Now())); !errors.Is(err, ErrNotFound) {
		t.Fatalf("create in unknown project: %v", err)
	}
	for _, id := range []string{"0123456789abcdef", "../../record", "0123456789ABCDEF"} {
		if _, err := v.Chat("desk", id); !errors.Is(err, ErrChatNotFound) {
			t.Fatalf("Chat(%q): %v", id, err)
		}
		if _, err := v.UpdateChat("desk", id, 0, func(*Chat) error { return nil }); !errors.Is(err, ErrChatNotFound) {
			t.Fatalf("UpdateChat(%q): %v", id, err)
		}
	}
	bad := newChat(time.Now())
	bad.Schema = "other/v1"
	if err := v.CreateChat("desk", bad); !errors.Is(err, ErrChatInvalid) {
		t.Fatalf("wrong schema: %v", err)
	}

	// A hand-edited or foreign file is skipped by the listing and reported by Chat.
	good := newChat(time.Now())
	if err := v.CreateChat("desk", good); err != nil {
		t.Fatal(err)
	}
	broken := "fedcba9876543210"
	if err := os.WriteFile(filepath.Join(root, "desk", "chats", broken+".json"), []byte(`{"schema":"ember.chat/v1"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "desk", "chats", "notes.txt"), []byte("ignored"), 0600); err != nil {
		t.Fatal(err)
	}
	if chats, err := v.Chats("desk"); err != nil || len(chats) != 1 || chats[0].ID != good.ID {
		t.Fatalf("Chats with a broken file = %+v, %v", chats, err)
	}
	if _, err := v.Chat("desk", broken); !errors.Is(err, ErrChatInvalid) {
		t.Fatalf("broken file: %v", err)
	}
}

func TestChatLimitAndSymlinks(t *testing.T) {
	v, root := chatVault(t)
	for range MaxChats {
		if err := v.CreateChat("desk", newChat(time.Now())); err != nil {
			t.Fatal(err)
		}
	}
	if err := v.CreateChat("desk", newChat(time.Now())); !errors.Is(err, ErrChatLimit) {
		t.Fatalf("chat %d: %v", MaxChats+1, err)
	}

	// chats/ must be a real directory inside the project, never a link elsewhere.
	outside := t.TempDir()
	c := newChat(time.Now())
	data, _ := encodeChat(&c)
	if err := os.WriteFile(filepath.Join(outside, c.ID+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "desk", "chats")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "desk", "chats")); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Chats("desk"); err == nil {
		t.Fatal("listed chats through a symlink")
	}
	if _, err := v.Chat("desk", c.ID); err == nil {
		t.Fatal("read a chat through a symlink")
	}
	if err := v.CreateChat("desk", newChat(time.Now())); err == nil {
		t.Fatal("wrote a chat through a symlink")
	}
}
