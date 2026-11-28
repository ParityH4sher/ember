package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ParityH4sher/ember/apps/api/internal/agents"
	"github.com/ParityH4sher/ember/apps/api/internal/harness"
	"github.com/ParityH4sher/ember/apps/api/internal/projects"
)

// Session runs (ADR 015). A chat with a session agent is one harness session
// on a worker. Ember appends the user's message and a pending agent message
// to the transcript, prompts the session and, when the worker reports that
// the run ended, copies the agent's text into the pending message. Messages
// sent while the run is active steer it.
//
// Locking: a run's mu is taken before ChatRuns.mu and the vault lock;
// ChatRuns.mu is taken inside the vault lock (as free() is for turn runs),
// never the other way round.

const (
	// maxWorkerRuns bounds the active session runs per worker.
	maxWorkerRuns = 4
	// maxSessionMessages is how much session history the API reads to copy
	// a run's answer.
	maxSessionMessages = 200
	// maxAnswer matches the router's 64 KiB output cap, which answerReserve
	// keeps room for.
	maxAnswer      = 64 << 10
	workerTimeout  = 15 * time.Second
	maxRetry       = 30 * time.Second
	keepalive      = 25 * time.Second
	sseWrite       = 5 * time.Second
	subscriberSize = 64
)

// Session run outcomes.
const (
	outcomeSucceeded   = "succeeded"
	outcomeFailed      = "failed"
	outcomeInterrupted = "interrupted"
	outcomeTimeout     = "timeout"
)

// sessionFailed is shown for a failed session run. The worker's own error
// text can contain provider responses and is never copied.
const sessionFailed = "The agent run failed. The operator can find details in the worker log."

var errNoChange = errors.New("no change")

// worker is a harness worker and its active run count (under ChatRuns.mu).
type worker struct {
	id        string
	h         harness.Harness
	directory string
	runs      int
}

// sessionRun is an active session run of one chat.
type sessionRun struct {
	key, project, chat string
	worker             *worker
	message            string // the pending agent message
	prompt             string // the run's first user message
	started            time.Time
	timer              *time.Timer

	// mu serialises the run's start, steers and end: starting and finishing
	// hold it exclusively, steers share it while they prompt. A steer that
	// gets it after the run ended starts a new run instead.
	mu sync.RWMutex
	// Under ChatRuns.mu:
	done    bool
	expired bool          // the deadline passed
	ended   chan struct{} // closed when done
}

func (s *sessionRun) sessionID() string { return harness.SessionID(s.chat) }

// claimSession returns the chat's active run, or creates one that the
// caller holds exclusively (created true) while it starts it.
func (r *ChatRuns) claimSession(key, projectID, chatID string, w *worker) (run *sessionRun, created bool, status int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, false, http.StatusServiceUnavailable
	}
	if run := r.sessions[key]; run != nil {
		return run, false, 0
	}
	if w.runs >= maxWorkerRuns {
		return nil, false, http.StatusTooManyRequests
	}
	run = &sessionRun{key: key, project: projectID, chat: chatID, worker: w, started: time.Now(), ended: make(chan struct{})}
	run.mu.Lock()
	r.sessions[key] = run
	w.runs++
	return run, true, 0
}

// releaseSession forgets the run. It is idempotent.
func (r *ChatRuns) releaseSession(run *sessionRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run.done {
		return
	}
	run.done = true
	close(run.ended)
	if run.timer != nil {
		run.timer.Stop()
	}
	if r.sessions[run.key] == run {
		delete(r.sessions, run.key)
	}
	run.worker.runs--
}

func (r *ChatRuns) isDone(run *sessionRun) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return run.done
}

// startTimer arms the run deadline. Call with run.mu held.
func (r *ChatRuns) startTimer(run *sessionRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run.done || r.closed {
		return
	}
	run.timer = time.AfterFunc(r.sessionDeadline, func() { r.expire(run) })
}

// sendSession starts a run, or steers the active one.
func (h *chatAPI) sendSession(w http.ResponseWriter, r *http.Request, chat projects.Chat, wk *worker, content string) {
	projectID, chatID := r.PathValue("projectId"), chat.ID
	key := chatKey(projectID, chatID)
	// The harness may already hold the prompt when the browser goes away;
	// finish the request so the transcript matches.
	ctx := context.WithoutCancel(r.Context())
	if !h.runs.running(key) && hasPending(chat) {
		h.runs.recoverSession(ctx, h.vaults, projectID, chat, wk)
	}
	// A steer that finds its run just ended starts a new run; a new run
	// raced by another request steers that one.
	for range 3 {
		run, created, status := h.runs.claimSession(key, projectID, chatID, wk)
		switch {
		case status == http.StatusServiceUnavailable:
			agentsUnavailable(w)
			return
		case status == http.StatusTooManyRequests:
			writeError(w, http.StatusTooManyRequests, "agent_busy", "the agent worker is busy with other chats; try again shortly")
			return
		case created:
			h.startSession(ctx, w, run, content)
			return
		}
		if h.steer(ctx, w, run, content) {
			return
		}
	}
	writeError(w, http.StatusConflict, "chat_busy", "the agent is still answering in this chat")
}

// startSession appends the user message and a pending agent message, opens
// the session and prompts it. It holds run.mu from claimSession.
func (h *chatAPI) startSession(ctx context.Context, w http.ResponseWriter, run *sessionRun, content string) {
	defer run.mu.Unlock()
	rs := h.runs
	now := rs.now()
	userID, pendingID := projects.NewID(), projects.NewID()
	titled := false
	saved, err := h.vaults.UpdateChat(run.project, run.chat, answerReserve, func(c *projects.Chat) error {
		// This request holds the chat, so any pending message is left over
		// from a run that no longer exists.
		markInterrupted(c)
		if c.Title == "" {
			c.Title, titled = titleFrom(content), true
		}
		c.Updated = now
		c.Messages = append(c.Messages,
			projects.ChatMessage{ID: userID, Role: "user", Content: content, Created: now},
			projects.ChatMessage{ID: pendingID, Role: "agent", Status: projects.StatusPending, Created: now})
		return nil
	})
	if err != nil {
		rs.releaseSession(run)
		chatError(w, err)
		return
	}
	run.message, run.prompt = pendingID, content
	sid := run.sessionID()
	err = run.worker.h.Open(ctx, harness.Session{ID: sid, Directory: run.worker.directory})
	if err == nil {
		err = run.worker.h.Prompt(ctx, sid, content)
	}
	if err != nil {
		// Nothing reached the agent: take the messages back.
		_, _ = h.vaults.UpdateChat(run.project, run.chat, 0, func(c *projects.Chat) error {
			c.Messages = removeMessages(c.Messages, userID, pendingID)
			if titled {
				c.Title = ""
			}
			return nil
		})
		rs.releaseSession(run)
		rs.logger.Warn("chat session not started", "chat", run.chat, "worker", run.worker.id, "error", err)
		workerUnavailable(w)
		return
	}
	rs.startTimer(run)
	rs.logger.Info("chat session run started", "chat", run.chat, "worker", run.worker.id)
	writeJSON(w, http.StatusAccepted, chatView{Chat: saved, Mode: modeSession, Running: true})
}

// steer adds a message to the active run. It returns false when the run
// ended before the message could be added; the caller then starts a new run.
func (h *chatAPI) steer(ctx context.Context, w http.ResponseWriter, run *sessionRun, content string) bool {
	run.mu.RLock()
	defer run.mu.RUnlock()
	if h.runs.isDone(run) {
		return false
	}
	now := h.runs.now()
	userID := projects.NewID()
	// Steer messages follow the pending agent message in the transcript; the
	// answer copied at the end of the run covers them.
	saved, err := h.vaults.UpdateChat(run.project, run.chat, answerReserve, func(c *projects.Chat) error {
		c.Updated = now
		c.Messages = append(c.Messages, projects.ChatMessage{ID: userID, Role: "user", Content: content, Created: now})
		return nil
	})
	if err != nil {
		chatError(w, err)
		return true
	}
	if err := run.worker.h.Prompt(ctx, run.sessionID(), content); err != nil {
		_, _ = h.vaults.UpdateChat(run.project, run.chat, 0, func(c *projects.Chat) error {
			c.Messages = removeMessages(c.Messages, userID)
			return nil
		})
		h.runs.logger.Warn("chat session not steered", "chat", run.chat, "worker", run.worker.id, "error", err)
		workerUnavailable(w)
		return true
	}
	writeJSON(w, http.StatusAccepted, chatView{Chat: saved, Mode: modeSession, Running: true})
	return true
}

func removeMessages(messages []projects.ChatMessage, ids ...string) []projects.ChatMessage {
	out := messages[:0]
	for _, m := range messages {
		drop := false
		for _, id := range ids {
			drop = drop || m.ID == id
		}
		if !drop {
			out = append(out, m)
		}
	}
	return out
}

// answer is what a session's history says about one run.
type answer struct {
	text      string // the agent text since the run's first user message
	found     bool   // the run's first user message is in the history
	idle      bool   // the newest message ends a run
	outcome   string // that run's outcome, when idle
	restarted bool   // a run ended after the prompt, and a later message started another
}

// readAnswer finds the run that prompt started in a session's history.
func readAnswer(msgs []harness.Message, prompt string) answer {
	var a answer
	start := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == harness.RoleUser && msgs[i].Text == prompt {
			start, a.found = i, true
			break
		}
	}
	if !a.found {
		// The prompt scrolled out of the window: take what follows the run
		// before the newest one.
		end := len(msgs)
		for end > 0 && msgs[end-1].Role == harness.RoleRun {
			end--
		}
		for i := end - 1; i >= 0; i-- {
			if msgs[i].Role == harness.RoleRun {
				start = i
				break
			}
		}
	}
	lastRun := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == harness.RoleRun {
			lastRun = i
			break
		}
	}
	if n := len(msgs); n > 0 && msgs[n-1].Role == harness.RoleRun {
		a.idle, a.outcome = true, outcomeOf(msgs[n-1].Text)
	}
	var parts []string
	for i := start + 1; i < len(msgs); i++ {
		switch {
		case msgs[i].Role == harness.RoleAgent && strings.TrimSpace(msgs[i].Text) != "":
			parts = append(parts, strings.TrimSpace(msgs[i].Text))
		case msgs[i].Role == harness.RoleUser && lastRun > start && i > lastRun:
			a.restarted = true
		}
	}
	a.text = clip(agents.CleanOutput(strings.Join(parts, "\n\n")), maxAnswer)
	return a
}

// outcomeOf maps the outcome OpenCode records at the end of a run.
func outcomeOf(s string) string {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "interrupt"), strings.Contains(s, "abort"), strings.Contains(s, "cancel"):
		return outcomeInterrupted
	case strings.Contains(s, "fail"), strings.Contains(s, "error"):
		return outcomeFailed
	}
	return outcomeSucceeded
}

func outcomeFromEvent(t string) string {
	switch t {
	case harness.RunSucceeded:
		return outcomeSucceeded
	case harness.RunInterrupted:
		return outcomeInterrupted
	}
	return outcomeFailed
}

// clip cuts s to at most n bytes at a character boundary.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return strings.TrimSpace(s[:n])
}

// applyOutcome records a session run's result in its pending message.
func applyOutcome(m *projects.ChatMessage, outcome, text string) {
	m.Content, m.Error = "", ""
	switch {
	case outcome == outcomeSucceeded && text != "":
		m.Status, m.Content = projects.StatusCompleted, text
	case outcome == outcomeSucceeded:
		m.Status, m.Error = projects.StatusFailed, agents.UserMessage(agents.CodeEmptyOutput)
	case outcome == outcomeInterrupted:
		m.Status, m.Error = projects.StatusFailed, agents.UserMessage(agents.CodeInterrupted)
	case outcome == outcomeTimeout:
		m.Status, m.Error = projects.StatusFailed, agents.UserMessage(agents.CodeTimeout)
	default:
		m.Status, m.Error = projects.StatusFailed, sessionFailed
	}
}

// finish copies the run's answer into the transcript and forgets the run.
// The worker reported the end (force false), or the API gave up waiting
// (force true, after the deadline). An end that a steer already followed
// with another run is ignored: that run's end finishes this one.
func (r *ChatRuns) finish(vaults *projects.Vaults, run *sessionRun, outcome string, force bool) bool {
	run.mu.Lock()
	defer run.mu.Unlock()
	if r.isDone(run) {
		return false
	}
	text := ""
	if !force {
		ctx, cancel := context.WithTimeout(r.ctx, workerTimeout)
		msgs, err := run.worker.h.Messages(ctx, run.sessionID(), maxSessionMessages)
		cancel()
		if err != nil {
			// Keep the run: the next subscription, recovery or the deadline
			// finishes it.
			r.logger.Warn("chat session answer not read", "chat", run.chat, "worker", run.worker.id, "error", err)
			return false
		}
		a := readAnswer(msgs, run.prompt)
		if a.restarted {
			return false
		}
		text = a.text
	}
	r.mu.Lock()
	if run.expired && outcome == outcomeInterrupted {
		outcome = outcomeTimeout
	}
	r.mu.Unlock()
	_, err := vaults.UpdateChat(run.project, run.chat, 0, func(c *projects.Chat) error {
		for i := range c.Messages {
			if m := &c.Messages[i]; m.ID == run.message && m.Status == projects.StatusPending {
				applyOutcome(m, outcome, text)
				c.Updated = r.now()
				r.releaseSession(run) // under the vault lock; see running
				return nil
			}
		}
		return errNoChange
	})
	r.releaseSession(run)
	if err != nil && !errors.Is(err, errNoChange) {
		r.logger.Error("chat session result not recorded", "chat", run.chat, "worker", run.worker.id, "error", err)
	}
	// Never log message text.
	r.logger.Info("chat session run", "chat", run.chat, "worker", run.worker.id, "outcome", outcome,
		"duration_ms", time.Since(run.started).Milliseconds(), "recorded", err == nil)
	return true
}

// expire interrupts a run that passed its deadline. The worker's
// run.interrupted event normally finishes it; if none comes, the run is
// recorded as timed out anyway.
func (r *ChatRuns) expire(run *sessionRun) {
	if !r.enter() {
		return
	}
	defer r.wg.Done()
	r.mu.Lock()
	run.expired = true
	r.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.ctx, workerTimeout)
	_, err := run.worker.h.Interrupt(ctx, run.sessionID())
	cancel()
	if err != nil {
		r.logger.Warn("chat session not interrupted after deadline", "chat", run.chat, "worker", run.worker.id, "error", err)
	}
	select {
	case <-run.ended:
		return
	case <-time.After(r.settle):
	case <-r.ctx.Done():
		return
	}
	if r.finish(r.vaults(), run, outcomeTimeout, true) {
		r.publish(run.project, run.chat, harness.Event{Type: harness.RunInterrupted})
	}
}

// recoverSession handles a pending message of a session chat that no run
// tracks, as after a restart. An idle session's answer is copied; a busy
// session's run is tracked again. When the worker cannot be asked, nothing
// changes and the caller shows the message as interrupted.
func (r *ChatRuns) recoverSession(ctx context.Context, vaults *projects.Vaults, projectID string, chat projects.Chat, w *worker) {
	pending := -1
	for i := len(chat.Messages) - 1; i >= 0; i-- {
		if chat.Messages[i].Role == "agent" && chat.Messages[i].Status == projects.StatusPending {
			pending = i
			break
		}
	}
	if pending < 0 {
		return
	}
	message, prompt := chat.Messages[pending].ID, ""
	for i := pending - 1; i >= 0; i-- {
		if chat.Messages[i].Role == "user" {
			prompt = chat.Messages[i].Content
			break
		}
	}
	ctx, cancel := context.WithTimeout(ctx, workerTimeout)
	msgs, err := w.h.Messages(ctx, harness.SessionID(chat.ID), maxSessionMessages)
	cancel()
	if err != nil {
		if !errors.Is(err, harness.ErrNotFound) {
			r.logger.Warn("chat session not recovered", "chat", chat.ID, "worker", w.id, "error", err)
		}
		return
	}
	a := readAnswer(msgs, prompt)
	key := chatKey(projectID, chat.ID)
	var adopted *sessionRun
	_, err = vaults.UpdateChat(projectID, chat.ID, 0, func(c *projects.Chat) error {
		// Under the vault lock: a run that started or ended since the read
		// above holds the chat or has replaced the pending message.
		var m *projects.ChatMessage
		for i := range c.Messages {
			if c.Messages[i].ID == message && c.Messages[i].Status == projects.StatusPending {
				m = &c.Messages[i]
			}
		}
		if m == nil || r.running(key) {
			return errNoChange
		}
		switch {
		case a.idle && !a.restarted && a.found:
			applyOutcome(m, a.outcome, a.text)
			c.Updated = r.now()
			return nil
		case a.idle && !a.found:
			// The prompt never reached the session (or scrolled out of
			// reach): the answer that is there belongs to an earlier run.
			applyOutcome(m, outcomeInterrupted, "")
			c.Updated = r.now()
			return nil
		}
		adopted = r.adopt(key, projectID, chat.ID, w, message, prompt)
		return errNoChange
	})
	if err != nil && !errors.Is(err, errNoChange) {
		r.logger.Error("chat session recovery not recorded", "chat", chat.ID, "worker", w.id, "error", err)
	}
	if adopted != nil {
		r.logger.Info("chat session run tracked again", "chat", chat.ID, "worker", w.id)
	}
}

// adopt tracks a run already active in the worker. It may exceed the
// worker's run limit: the run exists either way.
func (r *ChatRuns) adopt(key, projectID, chatID string, w *worker, message, prompt string) *sessionRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.sessions[key] != nil {
		return nil
	}
	run := &sessionRun{key: key, project: projectID, chat: chatID, worker: w, message: message, prompt: prompt,
		started: time.Now(), ended: make(chan struct{})}
	run.timer = time.AfterFunc(r.sessionDeadline, func() { r.expire(run) })
	r.sessions[key] = run
	w.runs++
	return run
}

// sessionFor returns the active run of chatID on worker w.
func (r *ChatRuns) sessionFor(w *worker, chatID string) *sessionRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, run := range r.sessions {
		if run.chat == chatID && run.worker == w {
			return run
		}
	}
	return nil
}

func (r *ChatRuns) workerRuns(w *worker) []*sessionRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*sessionRun
	for _, run := range r.sessions {
		if run.worker == w {
			out = append(out, run)
		}
	}
	return out
}

// follow holds the worker's one event subscription for the life of the
// process: it fans events out to chat subscribers, finishes runs, and
// resubscribes with backoff when the stream fails.
func (r *ChatRuns) follow(w *worker) {
	defer r.wg.Done()
	delay, failed := r.retry, false
	for {
		events, err := w.h.Subscribe(r.ctx)
		if err == nil {
			if failed {
				r.logger.Info("worker events resumed", "worker", w.id)
				// Browsers missed events while the stream was down.
				r.resync(w)
			}
			failed, delay = false, r.retry
			r.reconcile(w)
			for e := range events {
				r.dispatch(w, e)
			}
		}
		if r.ctx.Err() != nil {
			return
		}
		if !failed {
			r.logger.Warn("worker events unavailable", "worker", w.id)
			r.resync(w)
			failed = true
		}
		select {
		case <-r.ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, maxRetry)
	}
}

// reconcile finishes runs that ended while no subscription was open.
func (r *ChatRuns) reconcile(w *worker) {
	for _, run := range r.workerRuns(w) {
		if !r.enter() {
			return
		}
		go func() {
			defer r.wg.Done()
			ctx, cancel := context.WithTimeout(r.ctx, workerTimeout)
			msgs, err := w.h.Messages(ctx, run.sessionID(), maxSessionMessages)
			cancel()
			if err != nil {
				return
			}
			if a := readAnswer(msgs, run.prompt); a.idle && a.found && !a.restarted {
				if r.finish(r.vaults(), run, a.outcome, false) {
					r.publish(run.project, run.chat, harness.Event{Type: eventFor(a.outcome)})
				}
			}
		}()
	}
}

// dispatch handles one worker event. Run ends are forwarded after the
// transcript is written, so that a browser reloading on the event sees the
// answer.
func (r *ChatRuns) dispatch(w *worker, e harness.Event) {
	chatID, ok := harness.ChatID(e.Session)
	if !ok {
		return
	}
	if harness.RunEnded(e.Type) {
		if run := r.sessionFor(w, chatID); run != nil && r.enter() {
			go func() {
				defer r.wg.Done()
				r.finish(r.vaults(), run, outcomeFromEvent(e.Type), false)
				r.publishWorker(w, chatID, e)
			}()
			return
		}
	}
	r.publishWorker(w, chatID, e)
}

// vaults returns the vaults handed to the chat routes. Runs only exist once
// a route started them, so it is set by then.
func (r *ChatRuns) vaults() *projects.Vaults {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.store
}

// subscriber is one browser event stream of a chat.
type subscriber struct {
	project, worker string // worker is empty for a turn agent's chat
	ch              chan harness.Event
}

// subscribe adds an event stream for a chat, or returns nil once closing.
func (r *ChatRuns) subscribe(projectID, chatID, workerID string) *subscriber {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.streamsEnded {
		return nil
	}
	s := &subscriber{project: projectID, worker: workerID, ch: make(chan harness.Event, subscriberSize)}
	if r.subs[chatID] == nil {
		r.subs[chatID] = map[*subscriber]struct{}{}
	}
	r.subs[chatID][s] = struct{}{}
	return s
}

// EndStreams ends the browsers' event streams and refuses new ones, so that
// a graceful shutdown does not wait for them. Register it with
// http.Server.RegisterOnShutdown.
func (r *ChatRuns) EndStreams() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.streamsEnded = true
	for chatID, set := range r.subs {
		for s := range set {
			r.drop(chatID, s)
		}
	}
}

func (r *ChatRuns) unsubscribe(chatID string, s *subscriber) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drop(chatID, s)
}

// drop removes and closes a subscriber. Call with r.mu held.
func (r *ChatRuns) drop(chatID string, s *subscriber) {
	if _, ok := r.subs[chatID][s]; !ok {
		return
	}
	delete(r.subs[chatID], s)
	if len(r.subs[chatID]) == 0 {
		delete(r.subs, chatID)
	}
	close(s.ch)
}

// deliver queues e for the chat's subscribers that match. A subscriber that
// does not keep up is dropped; its browser reconnects and reloads.
func (r *ChatRuns) deliver(chatID string, e harness.Event, match func(*subscriber) bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for s := range r.subs[chatID] {
		if !match(s) {
			continue
		}
		select {
		case s.ch <- e:
		default:
			r.drop(chatID, s)
		}
	}
}

// publish sends an event to the subscribers of one project's chat.
func (r *ChatRuns) publish(projectID, chatID string, e harness.Event) {
	if r == nil {
		return
	}
	r.deliver(chatID, e, func(s *subscriber) bool { return s.project == projectID })
}

// publishWorker sends a worker event to the chat's subscribers on that worker.
func (r *ChatRuns) publishWorker(w *worker, chatID string, e harness.Event) {
	r.deliver(chatID, e, func(s *subscriber) bool { return s.worker == w.id })
}

// resync tells every browser following a chat on w to reload.
func (r *ChatRuns) resync(w *worker) {
	r.mu.Lock()
	var chats []string
	for chatID := range r.subs {
		chats = append(chats, chatID)
	}
	r.mu.Unlock()
	for _, chatID := range chats {
		r.publishWorker(w, chatID, harness.Event{Type: "resync"})
	}
}

// chatTarget loads the chat and its agent for the run routes.
func (h *chatAPI) chatTarget(w http.ResponseWriter, r *http.Request) (projects.Chat, chatAgent, bool) {
	if h.runs == nil {
		agentsUnavailable(w)
		return projects.Chat{}, chatAgent{}, false
	}
	chat, err := h.vaults.Chat(r.PathValue("projectId"), r.PathValue("chatId"))
	if err != nil {
		chatError(w, err)
		return projects.Chat{}, chatAgent{}, false
	}
	agent, ok := h.resolveAgent(chat.Agent)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "agents_unavailable", "this chat's agent is no longer offered on this instance")
		return projects.Chat{}, chatAgent{}, false
	}
	return chat, agent, true
}

// events streams a chat's live events as server-sent events.
func (h *chatAPI) events(w http.ResponseWriter, r *http.Request) {
	chat, agent, ok := h.chatTarget(w, r)
	if !ok {
		return
	}
	projectID, workerID := r.PathValue("projectId"), ""
	if agent.worker != nil {
		workerID = agent.worker.id
	}
	sub := h.runs.subscribe(projectID, chat.ID, workerID)
	if sub == nil {
		agentsUnavailable(w)
		return
	}
	defer h.runs.unsubscribe(chat.ID, sub)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	// Each write gets its own deadline, which also lifts the server's
	// WriteTimeout for this long-lived response.
	write := func(chunk string) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(sseWrite))
		if _, err := io.WriteString(w, chunk); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !write("data: {\"type\":\"connected\"}\n\n") {
		return
	}
	tick := time.NewTicker(keepalive)
	defer tick.Stop()
	for {
		select {
		case e, ok := <-sub.ch:
			if !ok {
				return
			}
			data, err := json.Marshal(e)
			if err != nil || !write("data: "+string(data)+"\n\n") {
				return
			}
		case <-tick.C:
			if !write(": keepalive\n\n") {
				return
			}
		case <-r.Context().Done():
			return
		case <-h.runs.ctx.Done():
			return
		}
	}
}

// interrupt stops the chat's active run.
func (h *chatAPI) interrupt(w http.ResponseWriter, r *http.Request) {
	chat, agent, ok := h.chatTarget(w, r)
	if !ok {
		return
	}
	key := chatKey(r.PathValue("projectId"), chat.ID)
	if agent.worker == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"interrupted": h.runs.interruptTurn(key)})
		return
	}
	// The worker's run.interrupted event finishes the run.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), workerTimeout)
	defer cancel()
	stopped, err := agent.worker.h.Interrupt(ctx, harness.SessionID(chat.ID))
	if err != nil {
		h.runs.logger.Warn("chat session not interrupted", "chat", chat.ID, "worker", agent.worker.id, "error", err)
		workerUnavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"interrupted": stopped})
}

// permissions lists the chat's open permission requests.
func (h *chatAPI) permissions(w http.ResponseWriter, r *http.Request) {
	chat, agent, ok := h.chatTarget(w, r)
	if !ok {
		return
	}
	list := []harness.Permission{}
	if agent.worker != nil {
		ctx, cancel := context.WithTimeout(r.Context(), workerTimeout)
		defer cancel()
		got, err := agent.worker.h.Permissions(ctx, harness.SessionID(chat.ID))
		if err != nil && !errors.Is(err, harness.ErrNotFound) {
			h.runs.logger.Warn("chat session permissions not listed", "chat", chat.ID, "worker", agent.worker.id, "error", err)
			workerUnavailable(w)
			return
		}
		if got != nil {
			list = got
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"permissions": list})
}

// reply answers one permission request.
func (h *chatAPI) reply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Decision string `json:"decision"`
	}
	if err := decodeJSON(w, r, &body); err != nil || (body.Decision != string(harness.Once) && body.Decision != string(harness.Reject)) {
		writeError(w, http.StatusBadRequest, "invalid_decision", `send a decision of "once" or "reject"`)
		return
	}
	chat, agent, ok := h.chatTarget(w, r)
	if !ok {
		return
	}
	permissionID := r.PathValue("permissionId")
	if agent.worker == nil || !permissionIDForm(permissionID) {
		permissionNotFound(w)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), workerTimeout)
	defer cancel()
	err := agent.worker.h.Reply(ctx, harness.SessionID(chat.ID), permissionID, harness.Decision(body.Decision))
	switch {
	case errors.Is(err, harness.ErrNotFound):
		permissionNotFound(w)
	case err != nil:
		h.runs.logger.Warn("chat session permission not answered", "chat", chat.ID, "worker", agent.worker.id, "error", err)
		workerUnavailable(w)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func permissionIDForm(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func permissionNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "permission_not_found", "this chat has no such permission request")
}

func workerUnavailable(w http.ResponseWriter) {
	writeError(w, http.StatusBadGateway, "worker_unavailable", "the agent worker is not reachable; try again shortly")
}

// eventFor is the event that reports a run outcome.
func eventFor(outcome string) string {
	switch outcome {
	case outcomeSucceeded:
		return harness.RunSucceeded
	case outcomeFailed:
		return harness.RunFailed
	}
	return harness.RunInterrupted
}

// useVaults hands the runs the transcripts the chat routes serve.
func (r *ChatRuns) useVaults(v *projects.Vaults) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store = v
}
