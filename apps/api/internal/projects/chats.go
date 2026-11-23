package projects

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/ParityH4sher/ember/apps/api/internal/projectfiles"
)

// ChatSchema identifies a project chat transcript file.
const ChatSchema = "ember.chat/v1"

const (
	// MaxChats bounds the chats in one project, and so the cost of listing them.
	MaxChats = 100
	// MaxChatBytes bounds one transcript file.
	MaxChatBytes = 1 << 20
)

var (
	ErrChatNotFound = errors.New("chat not found")
	ErrChatLimit    = errors.New("project has the maximum number of chats")
	ErrChatFull     = errors.New("chat reached its size limit")
	ErrChatInvalid  = errors.New("chat file is not a valid transcript")
)

// Agent message states. User messages have no status.
const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// ChatMessage is one message in a transcript.
type ChatMessage struct {
	ID      string    `json:"id"`
	Role    string    `json:"role"` // "user" or "agent"
	Content string    `json:"content"`
	Status  string    `json:"status,omitempty"`
	Error   string    `json:"error,omitempty"`
	Run     string    `json:"run,omitempty"`
	Created time.Time `json:"created"`
}

// Chat is a transcript stored as chats/<id>.json in the project directory.
// The file is plain JSON so it stays readable without Ember or any provider.
type Chat struct {
	Schema   string        `json:"schema"`
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	Agent    string        `json:"agent"`
	Created  time.Time     `json:"created"`
	Updated  time.Time     `json:"updated"`
	Messages []ChatMessage `json:"messages"`
}

var hexID = regexp.MustCompile(`^[0-9a-f]{16}$`)
var chatFile = regexp.MustCompile(`^[0-9a-f]{16}\.json$`)

// ValidChatID reports whether id has the form NewID produces.
func ValidChatID(id string) bool { return hexID.MatchString(id) }

// NewID returns 16 random lowercase hex characters for a chat or message.
func NewID() string {
	var b [8]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (c *Chat) valid(id string) bool {
	if c.Schema != ChatSchema || c.ID != id || !hexID.MatchString(c.ID) || c.Agent == "" {
		return false
	}
	for _, m := range c.Messages {
		if !hexID.MatchString(m.ID) {
			return false
		}
		switch {
		case m.Role == "user" && m.Status == "":
		case m.Role == "agent" && (m.Status == StatusPending || m.Status == StatusCompleted || m.Status == StatusFailed):
		default:
			return false
		}
	}
	return true
}

func encodeChat(c *Chat) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(c); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func readChat(store *projectfiles.Store, id string) (Chat, error) {
	f, err := store.Read(id + ".json")
	if errors.Is(err, fs.ErrNotExist) {
		return Chat{}, ErrChatNotFound
	}
	if err != nil {
		return Chat{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxChatBytes+1))
	if err != nil {
		return Chat{}, err
	}
	var c Chat
	if len(data) > MaxChatBytes || json.Unmarshal(data, &c) != nil || !c.valid(id) {
		return Chat{}, ErrChatInvalid
	}
	return c, nil
}

// chatStore opens the project's chats/ directory. With create false, a
// missing directory returns fs.ErrNotExist. projectfiles rejects a symlink.
func (v *Vaults) chatStore(projectID string, create bool) (*projectfiles.Store, error) {
	dir := filepath.Join(v.root, projectID, "chats")
	if create {
		if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	return projectfiles.Open(dir)
}

func (v *Vaults) chatIDs(projectID string) ([]string, error) {
	// Opening the store first refuses a chats/ that is a symlink.
	if _, err := v.chatStore(projectID, false); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(v.root, projectID, "chats"))
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.Type().IsRegular() && chatFile.MatchString(e.Name()) {
			ids = append(ids, e.Name()[:16])
		}
	}
	return ids, nil
}

// Chats returns the project's readable transcripts, most recently updated
// first. Files that are not valid transcripts are skipped.
func (v *Vaults) Chats(projectID string) ([]Chat, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.active(projectID) {
		return nil, ErrNotFound
	}
	ids, err := v.chatIDs(projectID)
	if err != nil || len(ids) == 0 {
		return []Chat{}, err
	}
	store, err := v.chatStore(projectID, false)
	if err != nil {
		return nil, err
	}
	chats := make([]Chat, 0, len(ids))
	for _, id := range ids {
		c, err := readChat(store, id)
		if errors.Is(err, ErrChatInvalid) || errors.Is(err, ErrChatNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		chats = append(chats, c)
	}
	sort.Slice(chats, func(i, j int) bool {
		if !chats[i].Updated.Equal(chats[j].Updated) {
			return chats[i].Updated.After(chats[j].Updated)
		}
		return chats[i].ID < chats[j].ID
	})
	return chats, nil
}

// Chat returns one transcript.
func (v *Vaults) Chat(projectID, chatID string) (Chat, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.active(projectID) {
		return Chat{}, ErrNotFound
	}
	if !ValidChatID(chatID) {
		return Chat{}, ErrChatNotFound
	}
	store, err := v.chatStore(projectID, false)
	if errors.Is(err, fs.ErrNotExist) {
		return Chat{}, ErrChatNotFound
	}
	if err != nil {
		return Chat{}, err
	}
	return readChat(store, chatID)
}

// CreateChat stores a new transcript. Its ID must be new.
func (v *Vaults) CreateChat(projectID string, c Chat) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.active(projectID) {
		return ErrNotFound
	}
	if !c.valid(c.ID) {
		return ErrChatInvalid
	}
	ids, err := v.chatIDs(projectID)
	if err != nil {
		return err
	}
	if len(ids) >= MaxChats {
		return ErrChatLimit
	}
	for _, id := range ids {
		if id == c.ID {
			return fs.ErrExist
		}
	}
	return v.writeChat(projectID, &c, 0)
}

// UpdateChat applies change to the stored transcript and saves it. The saved
// file must leave reserve bytes free below MaxChatBytes, so that room for an
// expected agent answer is claimed together with the message that asks for
// it. If change returns an error, nothing is saved.
func (v *Vaults) UpdateChat(projectID, chatID string, reserve int, change func(*Chat) error) (Chat, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.active(projectID) {
		return Chat{}, ErrNotFound
	}
	if !ValidChatID(chatID) {
		return Chat{}, ErrChatNotFound
	}
	store, err := v.chatStore(projectID, false)
	if errors.Is(err, fs.ErrNotExist) {
		return Chat{}, ErrChatNotFound
	}
	if err != nil {
		return Chat{}, err
	}
	c, err := readChat(store, chatID)
	if err != nil {
		return Chat{}, err
	}
	if err := change(&c); err != nil {
		return Chat{}, err
	}
	if !c.valid(chatID) {
		return Chat{}, ErrChatInvalid
	}
	return c, v.writeChat(projectID, &c, reserve)
}

func (v *Vaults) writeChat(projectID string, c *Chat, reserve int) error {
	data, err := encodeChat(c)
	if err != nil {
		return err
	}
	if len(data)+reserve > MaxChatBytes {
		return ErrChatFull
	}
	store, err := v.chatStore(projectID, true)
	if err != nil {
		return err
	}
	return store.Write(c.ID+".json", bytes.NewReader(data))
}
