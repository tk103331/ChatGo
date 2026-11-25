package a2ui

import (
	"encoding/json"
	"time"
)

// Action is a renderer-to-agent event produced by an interactive component.
type Action struct {
	Version           string         `json:"version,omitempty"`
	Name              string         `json:"name"`
	SurfaceID         string         `json:"surfaceId"`
	SourceComponentID string         `json:"sourceComponentId"`
	Timestamp         string         `json:"timestamp,omitempty"`
	UserMessage       string         `json:"userMessage,omitempty"`
	Context           map[string]any `json:"context,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

// NewAction builds an action for a component, timestamped now.
func NewAction(surfaceID, componentID, name string) Action {
	return Action{
		Version: Version, Name: name, SurfaceID: surfaceID,
		SourceComponentID: componentID,
		Timestamp:         time.Now().UTC().Format(time.RFC3339Nano),
		Context:           map[string]any{},
	}
}

// Envelope renders the action as the v1.0 renderer-to-agent envelope.
func (a Action) Envelope() map[string]any {
	version := a.Version
	if version == "" {
		version = Version
	}
	return map[string]any{"version": version, "action": a}
}

// CallAgentFunction asks the agent to run one of its functions.
type CallAgentFunction struct {
	Version        string         `json:"version,omitempty"`
	FunctionCallID string         `json:"functionCallId"`
	SurfaceID      string         `json:"surfaceId,omitempty"`
	CallFunction   map[string]any `json:"callFunction"`
}

// Envelope renders the call as the v1.0 renderer-to-agent envelope.
func (c CallAgentFunction) Envelope() map[string]any {
	version := c.Version
	if version == "" {
		version = Version
	}
	return map[string]any{"version": version, "callAgentFunction": c}
}

// RendererMessage is one renderer-to-agent event.
type RendererMessage struct {
	Action            *Action            `json:"action,omitempty"`
	CallAgentFunction *CallAgentFunction `json:"callAgentFunction,omitempty"`
	FunctionResponse  *FunctionResponse  `json:"rendererFunctionResponse,omitempty"`
	Error             *RendererError     `json:"error,omitempty"`
}

// FunctionResponse completes an agent-initiated renderer function call.
type FunctionResponse struct {
	FunctionCallID string         `json:"functionCallId"`
	Value          any            `json:"value,omitempty"`
	Error          *RendererError `json:"error,omitempty"`
}

// RendererError is a protocol or runtime error report.
type RendererError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// MarshalRendererMessage encodes one renderer-to-agent message with version
// 1.0, as a transport such as AG-UI or A2A carries it.
func MarshalRendererMessage(m RendererMessage) (json.RawMessage, error) {
	body, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	out["version"] = Version
	return json.Marshal(out)
}
