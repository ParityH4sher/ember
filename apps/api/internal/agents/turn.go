package agents

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
)

// TurnSchema identifies the chat-turn envelope an agent reads on stdin.
const TurnSchema = "ember.chat-turn/v1"

// MaxRequest is the sandbox router's body limit for POST /v1/runs. The
// envelope travels as a JSON string inside that body, so it is escaped twice.
const MaxRequest = 32 << 10

// ErrTooLarge means the newest message cannot reach the agent even when every
// earlier message is left out.
var ErrTooLarge = errors.New("message does not fit the sandbox router's input limit")

// Message is one entry of the conversation passed to the agent.
type Message struct {
	Role    string `json:"role"` // "user" or "agent"
	Content string `json:"content"`
}

// ProjectRef names the project a chat belongs to.
type ProjectRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ChatRef names the chat being continued.
type ChatRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// ModelRef names the model an admin chose for the agent. It carries no
// endpoint or credential: the agent asks the model gateway for this alias.
type ModelRef struct {
	ID           string `json:"id"`           // Ember alias, the gateway's model name
	Name         string `json:"name"`         // label people see
	ProviderKind string `json:"providerKind"` // anthropic, openai, ollama, openai-compatible
	Upstream     string `json:"upstream"`     // the provider's own model ID
	Local        bool   `json:"local"`        // served on the operator's network
}

// Turn is the document an agent receives on stdin for one chat turn. The last
// message is the one the agent should answer. It is data, not a grant: it
// cannot give the agent tools, files or network access.
type Turn struct {
	Schema  string     `json:"schema"`
	Project ProjectRef `json:"project"`
	Chat    ChatRef    `json:"chat"`
	// Model is present for agents configured by an admin with a model.
	Model    *ModelRef `json:"model,omitempty"`
	Messages []Message `json:"messages"`
	// OmittedMessages counts the oldest messages left out to fit the router's
	// input limit.
	OmittedMessages int `json:"omittedMessages"`
}

// Fit leaves out the fewest oldest messages needed for the router request to
// fit MaxRequest, and records how many it left out. The last message is always
// kept; if it cannot fit on its own, Fit returns ErrTooLarge.
func Fit(agent string, turn Turn) (Turn, error) {
	turn.Schema = TurnSchema
	all := turn.Messages
	if len(all) == 0 {
		return Turn{}, errors.New("a turn needs at least one message")
	}
	fits := func(drop int) bool {
		t := turn
		t.Messages, t.OmittedMessages = all[drop:], turn.OmittedMessages+drop
		body, err := encodeRequest(agent, t)
		return err == nil && len(body) <= MaxRequest
	}
	last := len(all) - 1
	if !fits(last) {
		return Turn{}, ErrTooLarge
	}
	// Leaving out more messages never makes the request larger, so the
	// smallest fitting drop count can be found by binary search.
	drop := sort.Search(last, fits)
	turn.Messages, turn.OmittedMessages = all[drop:], turn.OmittedMessages+drop
	return turn, nil
}

// encodeRequest produces the exact POST /v1/runs body for this turn. HTML
// escaping is off so that "<" and "&" do not grow sixfold at each level.
func encodeRequest(agent string, turn Turn) ([]byte, error) {
	input, err := marshal(turn)
	if err != nil {
		return nil, err
	}
	return marshal(struct {
		Agent string `json:"agent"`
		Input string `json:"input"`
	}{agent, string(input)})
}

func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
