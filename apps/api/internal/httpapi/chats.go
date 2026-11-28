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
	"github.com/ParityH4sher/ember/apps/api/internal/aicatalog"
	"github.com/ParityH4sher/ember/apps/api/internal/harness"
	"github.com/ParityH4sher/ember/apps/api/internal/projects"
)

const (
	maxChatMessage = 16 << 10
	maxChatTitle   = 120
	// answerReserve keeps room in the transcript for the agent's answer (the
	// router caps stdout at 64 KiB) plus its JSON encoding.
	answerReserve = 72 << 10
	runDeadline   = 90 * time.Second
	// sessionDeadline bounds a session run (ADR 015).
	sessionDeadline = 15 * time.Minute
	sessionSettle   = 10 * time.Second
)

// ChatRuns starts chat runs and tracks the ones in progress. Turn agents run
// once per message in the sandbox router; session agents are long-lived
// harness sessions on a worker (ADR 015). Create one per process, pass it to
// NewDeploymentHandler with WithChatRuns, and Close it after the HTTP server
// has stopped.
//
// Run state is held in memory only. It is not a durable task record: a turn
// run that is in progress when the process stops is reported as
// interrupted; a session run keeps going in its worker and is picked up
// again by the next GET of its chat.
type ChatRuns struct {
	config   *agents.Config
	runner   agents.Runner // nil without a router socket
	logger   *slog.Logger
	now      func() time.Time
	deadline time.Duration // turn runs
	// sessionDeadline bounds a session run; then the API interrupts it.
	sessionDeadline time.Duration
	// settle is how long an interrupted session run may take to report its
	// end before the API records it as failed.
	settle time.Duration
	// retry is the first delay before resubscribing to a worker's events.
	retry   time.Duration
	workers map[string]*worker

	ctx    context.Context
	cancel context.CancelFunc
	slot   chan struct{} // the router runs one job at a time
	wg     sync.WaitGroup

	mu           sync.Mutex
	closed       bool
	streamsEnded bool
	active       map[string]context.CancelFunc       // turn runs by projectID + "/" + chatID
	sessions     map[string]*sessionRun              // session runs by chat key
	subs         map[string]map[*subscriber]struct{} // browser streams by chat ID
	store        *projects.Vaults
}

// RunOption configures NewChatRuns.
type RunOption func(*ChatRuns)

// WithHarness serves the session agents of the configured worker id with h.
// A worker without a harness offers no chats.
func WithHarness(id string, h harness.Harness) RunOption {
	return func(r *ChatRuns) {
		if w, ok := r.config.Worker(id); ok && h != nil {
			r.workers[id] = &worker{id: id, h: h, directory: w.Directory}
		}
	}
}

// NewChatRuns runs the configured agents: turn agents with runner, which may
// be nil when no router is configured, and session agents with the harnesses
// given by WithHarness. It subscribes to each worker's events at once.
func NewChatRuns(config *agents.Config, runner agents.Runner, logger *slog.Logger, opts ...RunOption) *ChatRuns {
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &ChatRuns{
		config: config, runner: runner, logger: logger,
		now: func() time.Time { return time.Now().UTC() }, deadline: runDeadline,
		sessionDeadline: sessionDeadline, settle: sessionSettle, retry: time.Second,
		workers: map[string]*worker{},
		ctx:     ctx, cancel: cancel, slot: make(chan struct{}, 1),
		active: map[string]context.CancelFunc{}, sessions: map[string]*sessionRun{},
		subs: map[string]map[*subscriber]struct{}{},
	}
	for _, opt := range opts {
		opt(r)
	}
	for _, w := range r.workers {
		r.wg.Add(1)
		go r.follow(w)
	}
	return r
}

// Close cancels turn runs in progress, which stops their containers, stops
// following workers, and waits up to wait for runs to record their outcome.
// Session runs keep going in their workers.
func (r *ChatRuns) Close(wait time.Duration) {
	r.mu.Lock()
	r.closed = true
	for _, run := range r.sessions {
		if run.timer != nil {
			run.timer.Stop()
		}
	}
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

// enter registers background work that Close waits for. It fails once the
// runs are closing.
func (r *ChatRuns) enter() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return false
	}
	r.wg.Add(1)
	return true
}

func chatKey(projectID, chatID string) string { return projectID + "/" + chatID }

// begin claims a chat for one turn run and returns the run's context. It
// fails if the chat already has a run or the runner is closing. Every
// successful begin is paired with one release and one wg.Done; end does both
// for a run that never started.
func (r *ChatRuns) begin(key string) (ctx context.Context, ok bool, closed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, false, true
	}
	if _, busy := r.active[key]; busy || r.sessions[key] != nil {
		return nil, false, false
	}
	ctx, cancel := context.WithTimeout(r.ctx, r.deadline)
	r.active[key] = cancel
	r.wg.Add(1)
	return ctx, true, false
}

func (r *ChatRuns) release(key string) {
	r.mu.Lock()
	if cancel := r.active[key]; cancel != nil {
		cancel()
	}
	delete(r.active, key)
	r.mu.Unlock()
}

func (r *ChatRuns) end(key string) {
	r.release(key)
	r.wg.Done()
}

// interruptTurn cancels the chat's turn run, if it has one.
func (r *ChatRuns) interruptTurn(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	cancel, ok := r.active[key]
	if ok {
		cancel()
	}
	return ok
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
	_, turn := r.active[key]
	return turn || r.sessions[key] != nil
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
	for key := range r.sessions {
		running[key] = true
	}
	return running
}

// chatAPI serves the chat routes. runs is nil when no agents are configured;
// transcripts stay readable.
type chatAPI struct {
	vaults   *projects.Vaults
	runs     *ChatRuns
	catalog  *aicatalog.Store // agents added by admins; may be nil
	projects projects.Store
}

// Agent modes: a turn agent runs once per message in the sandbox router; a
// session agent is a long-lived harness session (ADR 015).
const (
	modeTurn    = "turn"
	modeSession = "session"
)

// agentInfo is an agent as GET /v1/agents lists it.
type agentInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Mode    string `json:"mode,omitempty"`
	Harness string `json:"harness,omitempty"`
	Model   string `json:"model,omitempty"`
}

// chatAgent is what a chat run needs to know about its agent.
type chatAgent struct {
	router string           // policy name in the sandbox router (turn agents)
	model  *agents.ModelRef // nil when no model is configured
	worker *worker          // the harness worker (session agents)
}

func (a chatAgent) mode() string {
	if a.worker != nil {
		return modeSession
	}
	return modeTurn
}

// resolveAgent finds an agent in the operator's agents file, then among the
// enabled agents an admin added. A session agent whose worker has no harness
// is not offered.
func (h *chatAPI) resolveAgent(id string) (chatAgent, bool) {
	if h.runs == nil {
		return chatAgent{}, false
	}
	if a, ok := h.runs.config.Agent(id); ok {
		if a.Worker == "" {
			return chatAgent{router: a.ID}, true
		}
		w := h.runs.workers[a.Worker]
		return chatAgent{worker: w}, w != nil
	}
	if h.catalog == nil {
		return chatAgent{}, false
	}
	r, ok := h.catalog.Agent(id)
	if !ok {
		return chatAgent{}, false
	}
	// A catalog agent with a worker is a session agent; the worker's own
	// configuration chooses its model (ADR 015).
	if r.Agent.Worker != "" {
		w := h.runs.workers[r.Agent.Worker]
		return chatAgent{worker: w}, w != nil
	}
	out := chatAgent{router: r.Agent.RouterAgent}
	if r.Model != nil {
		out.model = &agents.ModelRef{ID: r.Model.ID, Name: r.Model.Name}
	}
	return out, true
}

func (h *chatAPI) listAgents(w http.ResponseWriter, _ *http.Request) {
	list := []agentInfo{}
	if h.runs != nil {
		for _, a := range h.runs.config.Agents {
			mode := modeTurn
			if a.Worker != "" {
				mode = modeSession
			}
			list = append(list, agentInfo{ID: a.ID, Name: a.Name, Mode: mode})
		}
		if h.catalog != nil {
			models := map[string]string{}
			for _, m := range h.catalog.View().Models {
				models[m.ID] = m.Name
			}
			for _, a := range h.catalog.EnabledAgents() {
				mode := modeTurn
				if a.Worker != "" {
					if h.runs.workers[a.Worker] == nil {
						continue // not offered; see resolveAgent
					}
					mode = modeSession
				}
				list = append(list, agentInfo{ID: a.ID, Name: a.Name, Mode: mode, Harness: a.Harness, Model: models[a.DefaultModel]})
			}
		}
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
	if _, ok := h.resolveAgent(body.Agent); !ok {
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

// chatView is a chat as GET and POST messages return it.
type chatView struct {
	projects.Chat
	// Mode is the agent's mode, or empty when the agent is no longer offered.
	Mode    string `json:"mode,omitempty"`
	Running bool   `json:"running"`
}

func (h *chatAPI) get(w http.ResponseWriter, r *http.Request) {
	projectID, chatID := r.PathValue("projectId"), r.PathValue("chatId")
	key := chatKey(projectID, chatID)
	running := h.runs.running(key) // before reading; see running
	chat, err := h.vaults.Chat(projectID, chatID)
	if err != nil {
		chatError(w, err)
		return
	}
	agent, known := h.resolveAgent(chat.Agent)
	if !running && known && agent.worker != nil && hasPending(chat) {
		// The API restarted while a session run was active: ask the worker.
		h.runs.recoverSession(r.Context(), h.vaults, projectID, chat, agent.worker)
		running = h.runs.running(key)
		if chat, err = h.vaults.Chat(projectID, chatID); err != nil {
			chatError(w, err)
			return
		}
	}
	if !running {
		markInterrupted(&chat)
	}
	view := chatView{Chat: chat, Running: running}
	if known {
		view.Mode = agent.mode()
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, view)
}

func hasPending(c projects.Chat) bool {
	for _, m := range c.Messages {
		if m.Status == projects.StatusPending {
			return true
		}
	}
	return false
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
	agent, ok := h.resolveAgent(chat.Agent)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "agents_unavailable", "this chat's agent is no longer offered on this instance")
		return
	}
	if agent.worker != nil {
		h.sendSession(w, r, chat, agent.worker, content)
		return
	}

	key := chatKey(projectID, chatID)
	ctx, ok, closed := h.runs.begin(key)
	if !ok {
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
		fitted, err := agents.Fit(agent.router, agents.Turn{
			Project:  agents.ProjectRef{ID: projectID, Name: h.projectName(projectID)},
			Chat:     agents.ChatRef{ID: c.ID, Title: c.Title},
			Model:    agent.model,
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
	h.runs.publish(projectID, chatID, harness.Event{Type: harness.RunStarted})
	go h.runs.run(ctx, h.vaults, projectID, chatID, pendingID, agent.router, turn)
	writeJSON(w, http.StatusAccepted, chatView{Chat: saved, Mode: modeTurn, Running: true})
}

// run waits for the agent and records its answer or a failure in place of
// the pending message. It owns the slot and the chat claim taken by send.
//
// It gives both back while it holds the vault lock to record the result, so
// that a client which sees the answer can send the next message at once:
// that send, and every read, waits for the vault lock and sees the result.
// Close waits for wg, which is released only after the write.
func (r *ChatRuns) run(ctx context.Context, vaults *projects.Vaults, projectID, chatID, messageID, agentID string, turn agents.Turn) {
	key := chatKey(projectID, chatID)
	defer r.wg.Done()
	var once sync.Once
	free := func() { once.Do(func() { <-r.slot; r.release(key) }) }
	defer free() // if the result could not be recorded
	started := time.Now()
	var result agents.Result
	var err error
	if r.runner == nil {
		err = &agents.RunError{Code: agents.CodeRouterUnavailable}
	} else {
		result, err = r.runner.Run(ctx, agentID, turn)
	}
	outcome, runID := "completed", result.RunID
	if err != nil {
		outcome = agents.CodeFailed
		var runErr *agents.RunError
		if errors.As(err, &runErr) {
			outcome, runID = runErr.Code, runErr.RunID
		}
		// Cancelled by Close or by POST interrupt, not by the deadline.
		if r.ctx.Err() != nil || errors.Is(ctx.Err(), context.Canceled) {
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
	event := harness.RunFailed
	switch outcome {
	case "completed":
		event = harness.RunSucceeded
	case agents.CodeInterrupted:
		event = harness.RunInterrupted
	}
	r.publish(projectID, chatID, harness.Event{Type: event})
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
