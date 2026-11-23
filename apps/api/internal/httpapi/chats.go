package httpapi

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ParityH4sher/ember/apps/api/internal/agents"
	"github.com/ParityH4sher/ember/apps/api/internal/projects"
)

const (
	maxChatMessage = 16 << 10
	maxChatTitle   = 120
	// answerReserve keeps room in the transcript for the agent's answer (the
	// router caps stdout at 64 KiB) plus its JSON encoding.
	answerReserve = 72 << 10
	runDeadline   = 90 * time.Second
)

// ChatRuns starts chat turns in the sandbox router and tracks the ones in
// progress. Create one per process, pass it to NewDeploymentHandler with
// WithChatRuns, and Close it after the HTTP server has stopped.
//
// Run state is held in memory only. It is not a durable task record: a run
// that is in progress when the process stops is reported as interrupted.
type ChatRuns struct {
	config   *agents.Config
	runner   agents.Runner
	logger   *slog.Logger
	now      func() time.Time
	deadline time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	slot   chan struct{} // the router runs one job at a time
	wg     sync.WaitGroup

	mu     sync.Mutex
	closed bool
	active map[string]bool // projectID + "/" + chatID
}

// NewChatRuns runs the configured agents with runner.
func NewChatRuns(config *agents.Config, runner agents.Runner, logger *slog.Logger) *ChatRuns {
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ChatRuns{
		config: config, runner: runner, logger: logger,
		now: func() time.Time { return time.Now().UTC() }, deadline: runDeadline,
		ctx: ctx, cancel: cancel, slot: make(chan struct{}, 1), active: map[string]bool{},
	}
}

// Close cancels runs in progress, which stops their containers, and waits up
// to wait for them to record the interruption.
func (r *ChatRuns) Close(wait time.Duration) {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	r.cancel()
	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(wait):
		r.logger.Warn("chat runs still recording after shutdown wait")
	}
}

func chatKey(projectID, chatID string) string { return projectID + "/" + chatID }

// begin claims a chat for one run. It fails if the chat already has a run or
// the runner is closing. Every successful begin is paired with one release
// and one wg.Done; end does both for a run that never started.
func (r *ChatRuns) begin(key string) (ok bool, closed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return false, true
	}
	if r.active[key] {
		return false, false
	}
	r.active[key] = true
	r.wg.Add(1)
	return true, false
}

func (r *ChatRuns) release(key string) {
	r.mu.Lock()
	delete(r.active, key)
	r.mu.Unlock()
}

func (r *ChatRuns) end(key string) {
	r.release(key)
	r.wg.Done()
}

// running reports whether a run holds the chat. Readers ask before reading
// the transcript: a run releases its claim only while it writes its result
// under the vault lock, so a reader that saw the claim and then reads a
// pending message is never looking at a finished run.
func (r *ChatRuns) running(key string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active[key]
}

func (r *ChatRuns) runningNow() map[string]bool {
	running := map[string]bool{}
	if r == nil {
		return running
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for key := range r.active {
		running[key] = true
	}
	return running
}

// chatAPI serves the chat routes. runs is nil when no agents are configured;
// transcripts stay readable.
type chatAPI struct {
	vaults   *projects.Vaults
	runs     *ChatRuns
	projects projects.Store
}

func (h *chatAPI) listAgents(w http.ResponseWriter, _ *http.Request) {
	list := []agents.Agent{}
	if h.runs != nil {
		list = append(list, h.runs.config.Agents...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": h.runs != nil, "agents": list})
}

type chatSummary struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Agent        string    `json:"agent"`
	Created      time.Time `json:"created"`
	Updated      time.Time `json:"updated"`
	MessageCount int       `json:"messageCount"`
	Pending      bool      `json:"pending"`
}

func (h *chatAPI) list(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	running := h.runs.runningNow() // before reading; see running
	chats, err := h.vaults.Chats(projectID)
	if err != nil {
		chatError(w, err)
		return
	}
	summaries := make([]chatSummary, 0, len(chats))
	for _, c := range chats {
		summaries = append(summaries, chatSummary{
			ID: c.ID, Title: c.Title, Agent: c.Agent, Created: c.Created, Updated: c.Updated,
			MessageCount: len(c.Messages), Pending: running[chatKey(projectID, c.ID)],
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"chats": summaries})
}

func (h *chatAPI) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Agent string  `json:"agent"`
		Title *string `json:"title"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_chat", "send an agent ID and an optional title")
		return
	}
	title := ""
	if body.Title != nil {
		title = strings.TrimSpace(*body.Title)
		if title != "" && !singleLine(title, maxChatTitle) {
			writeError(w, http.StatusBadRequest, "invalid_chat", "use a single-line title of at most 120 bytes")
			return
		}
	}
	if h.runs == nil {
		agentsUnavailable(w)
		return
	}
	if _, ok := h.runs.config.Agent(body.Agent); !ok {
		writeError(w, http.StatusBadRequest, "unknown_agent", "choose an agent listed by /v1/agents")
		return
	}
	now := h.runs.now()
	chat := projects.Chat{
		Schema: projects.ChatSchema, ID: projects.NewID(), Title: title, Agent: body.Agent,
		Created: now, Updated: now, Messages: []projects.ChatMessage{},
	}
	if err := h.vaults.CreateChat(r.PathValue("projectId"), chat); err != nil {
		chatError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, chat)
}

func (h *chatAPI) get(w http.ResponseWriter, r *http.Request) {
	projectID, chatID := r.PathValue("projectId"), r.PathValue("chatId")
	running := h.runs.running(chatKey(projectID, chatID)) // before reading; see running
	chat, err := h.vaults.Chat(projectID, chatID)
	if err != nil {
		chatError(w, err)
		return
	}
	if !running {
		markInterrupted(&chat)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, chat)
}

// markInterrupted fails pending messages whose run no longer exists, for
// example because the API restarted while the agent was working.
func markInterrupted(c *projects.Chat) {
	for i := range c.Messages {
		if m := &c.Messages[i]; m.Status == projects.StatusPending {
			m.Status, m.Error = projects.StatusFailed, agents.UserMessage(agents.CodeInterrupted)
		}
	}
}

func (h *chatAPI) send(w http.ResponseWriter, r *http.Request) {
	projectID, chatID := r.PathValue("projectId"), r.PathValue("chatId")
	var body struct {
		Content *string `json:"content"`
	}
	if err := decodeJSON(w, r, &body); err != nil || body.Content == nil {
		writeError(w, http.StatusBadRequest, "invalid_message", "send the message as content")
		return
	}
	content := strings.TrimSpace(*body.Content)
	if len(content) > maxChatMessage {
		writeError(w, http.StatusRequestEntityTooLarge, "message_too_large", "messages are limited to 16 KiB")
		return
	}
	if content == "" || !utf8.ValidString(content) || strings.IndexByte(content, 0) >= 0 {
		writeError(w, http.StatusBadRequest, "invalid_message", "send non-empty UTF-8 text")
		return
	}
	if h.runs == nil {
		agentsUnavailable(w)
		return
	}
	chat, err := h.vaults.Chat(projectID, chatID)
	if err != nil {
		chatError(w, err)
		return
	}
	agent, ok := h.runs.config.Agent(chat.Agent)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "agents_unavailable", "this chat's agent is no longer offered on this instance")
		return
	}

	key := chatKey(projectID, chatID)
	if ok, closed := h.runs.begin(key); !ok {
		if closed {
			agentsUnavailable(w)
		} else {
			writeError(w, http.StatusConflict, "chat_busy", "the agent is still answering in this chat")
		}
		return
	}
	select {
	case h.runs.slot <- struct{}{}:
	default:
		h.runs.end(key)
		writeError(w, http.StatusTooManyRequests, "agent_busy", "the agent runner is busy with another chat; try again shortly")
		return
	}
	started := false
	defer func() {
		if !started {
			<-h.runs.slot
			h.runs.end(key)
		}
	}()

	now := h.runs.now()
	pendingID := projects.NewID()
	var turn agents.Turn
	saved, err := h.vaults.UpdateChat(projectID, chatID, answerReserve, func(c *projects.Chat) error {
		// This request holds the chat, so any pending message is left over
		// from a run that no longer exists.
		markInterrupted(c)
		if c.Title == "" {
			c.Title = titleFrom(content)
		}
		fitted, err := agents.Fit(agent.ID, agents.Turn{
			Project:  agents.ProjectRef{ID: projectID, Name: h.projectName(projectID)},
			Chat:     agents.ChatRef{ID: c.ID, Title: c.Title},
			Messages: append(history(c.Messages), agents.Message{Role: "user", Content: content}),
		})
		if err != nil {
			return err
		}
		turn = fitted
		c.Updated = now
		c.Messages = append(c.Messages,
			projects.ChatMessage{ID: projects.NewID(), Role: "user", Content: content, Created: now},
			projects.ChatMessage{ID: pendingID, Role: "agent", Status: projects.StatusPending, Created: now})
		return nil
	})
	if err != nil {
		chatError(w, err)
		return
	}
	started = true
	go h.runs.run(h.vaults, projectID, chatID, pendingID, agent.ID, turn)
	writeJSON(w, http.StatusAccepted, saved)
}

// run waits for the agent and records its answer or a failure in place of
// the pending message. It owns the slot and the chat claim taken by send.
//
// It gives both back while it holds the vault lock to record the result, so
// that a client which sees the answer can send the next message at once:
// that send, and every read, waits for the vault lock and sees the result.
// Close waits for wg, which is released only after the write.
func (r *ChatRuns) run(vaults *projects.Vaults, projectID, chatID, messageID, agentID string, turn agents.Turn) {
	key := chatKey(projectID, chatID)
	defer r.wg.Done()
	var once sync.Once
	free := func() { once.Do(func() { <-r.slot; r.release(key) }) }
	defer free() // if the result could not be recorded
	ctx, cancel := context.WithTimeout(r.ctx, r.deadline)
	defer cancel()
	started := time.Now()
	result, err := r.runner.Run(ctx, agentID, turn)
	outcome, runID := "completed", result.RunID
	if err != nil {
		outcome = agents.CodeFailed
		var runErr *agents.RunError
		if errors.As(err, &runErr) {
			outcome, runID = runErr.Code, runErr.RunID
		}
		if r.ctx.Err() != nil {
			outcome = agents.CodeInterrupted
		}
	}
	_, saveErr := vaults.UpdateChat(projectID, chatID, 0, func(c *projects.Chat) error {
		for i := range c.Messages {
			m := &c.Messages[i]
			if m.ID != messageID {
				continue
			}
			m.Run = runID
			if err != nil {
				m.Status, m.Error = projects.StatusFailed, agents.UserMessage(outcome)
			} else {
				m.Status, m.Content = projects.StatusCompleted, result.Output
			}
			c.Updated = r.now()
			free()
			return nil
		}
		return projects.ErrChatNotFound
	})
	// Never log message text or the router's error body.
	r.logger.Info("chat run", "chat", chatID, "agent", agentID, "run", runID, "outcome", outcome,
		"omitted_messages", turn.OmittedMessages, "duration_ms", time.Since(started).Milliseconds(),
		"recorded", saveErr == nil)
	if saveErr != nil {
		r.logger.Error("chat run result not recorded", "chat", chatID, "run", runID, "error", saveErr)
	}
}

// history is the conversation the agent sees: user messages and answers it
// completed. Failed and pending turns are left out.
func history(messages []projects.ChatMessage) []agents.Message {
	out := make([]agents.Message, 0, len(messages)+1)
	for _, m := range messages {
		switch {
		case m.Role == "user":
			out = append(out, agents.Message{Role: "user", Content: m.Content})
		case m.Role == "agent" && m.Status == projects.StatusCompleted:
			out = append(out, agents.Message{Role: "agent", Content: m.Content})
		}
	}
	return out
}

func (h *chatAPI) projectName(id string) string {
	for _, p := range h.projects.List() {
		if p.ID == id {
			return p.Name
		}
	}
	return ""
}

// titleFrom uses the start of the first line of the first message.
func titleFrom(content string) string {
	line, _, _ := strings.Cut(content, "\n")
	line = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(line))
	if utf8.RuneCountInString(line) <= 60 {
		return line
	}
	return strings.TrimSpace(string([]rune(line)[:60])) + "…"
}

func singleLine(s string, maxBytes int) bool {
	if len(s) > maxBytes || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func agentsUnavailable(w http.ResponseWriter) {
	writeError(w, http.StatusServiceUnavailable, "agents_unavailable", "chat runs are not enabled on this Ember instance")
}

func chatError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, projects.ErrNotFound):
		writeError(w, http.StatusNotFound, "project_not_found", "project does not exist")
	case errors.Is(err, projects.ErrChatNotFound):
		writeError(w, http.StatusNotFound, "chat_not_found", "chat does not exist")
	case errors.Is(err, projects.ErrChatLimit):
		writeError(w, http.StatusRequestEntityTooLarge, "chat_limit", "this project has the maximum of 100 chats")
	case errors.Is(err, projects.ErrChatFull):
		writeError(w, http.StatusRequestEntityTooLarge, "chat_full", "this chat reached its size limit; start a new chat")
	case errors.Is(err, agents.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "message_too_large", "the message is too large to send to the agent")
	case errors.Is(err, projects.ErrChatInvalid), errors.Is(err, fs.ErrInvalid):
		writeError(w, http.StatusInternalServerError, "storage_error", "the chat file cannot be read")
	default:
		writeError(w, http.StatusInternalServerError, "storage_error", "could not access project chats")
	}
}
