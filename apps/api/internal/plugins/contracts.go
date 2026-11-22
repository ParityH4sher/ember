package plugins

import "context"

// Definition is the versioned, frontend-visible contract for an included plugin.
type Definition struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Version             string         `json:"version"`
	APIVersion          string         `json:"apiVersion"`
	Description         string         `json:"description,omitempty"`
	Capabilities        []string       `json:"capabilities"`
	ConfigurationSchema map[string]any `json:"configurationSchema"`
	Operations          []Operation    `json:"-"`
}

type Operation struct {
	OperationID  string         `json:"operationId"`
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	Method       string         `json:"method"`
	Path         string         `json:"path"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema"`
}

type Adapter interface {
	Invoke(context.Context, string, map[string]any, map[string]any) (any, error)
}

type Registration struct {
	Definition Definition
	Adapter    Adapter
}
