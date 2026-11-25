// Package a2ui implements the protocol half of A2UI 1.0: envelopes,
// surfaces, the data model and its bindings. It does not render anything and
// does not depend on MyGo, so a native, web or test renderer can share it.
package a2ui

import (
	"encoding/json"
	"fmt"
)

// Version is the A2UI revision this package speaks.
const Version = "1.0"

// Known catalog identifiers. A catalogId is an opaque string, so these are
// conveniences rather than the only accepted values.
const (
	BasicCatalogID = "https://a2ui.org/specification/v1_0/catalogs/basic/catalog.json"
)

// Message is one envelope. Exactly one payload field is set.
type Message struct {
	Version               string                 `json:"version,omitempty"`
	CreateSurface         *CreateSurface         `json:"createSurface,omitempty"`
	UpdateComponents      *UpdateComponents      `json:"updateComponents,omitempty"`
	UpdateDataModel       *UpdateDataModel       `json:"updateDataModel,omitempty"`
	DeleteSurface         *DeleteSurface         `json:"deleteSurface,omitempty"`
	CallRendererFunction  *CallRendererFunction  `json:"callRendererFunction,omitempty"`
	AgentFunctionResponse *AgentFunctionResponse `json:"agentFunctionResponse,omitempty"`
}

// CreateSurface starts a surface. Components and DataModel let an agent send
// the whole tree in this one message, as v1.0 allows.
type CreateSurface struct {
	SurfaceID     string         `json:"surfaceId"`
	CatalogID     string         `json:"catalogId,omitempty"`
	SendDataModel bool           `json:"sendDataModel,omitempty"`
	Components    []Component    `json:"components,omitempty"`
	DataModel     map[string]any `json:"dataModel,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// UpdateComponents adds or replaces components on a surface.
type UpdateComponents struct {
	SurfaceID  string      `json:"surfaceId"`
	Components []Component `json:"components"`
}

// UpdateDataModel replaces the value at Path; Path defaults to "/".
type UpdateDataModel struct {
	SurfaceID string `json:"surfaceId"`
	Path      string `json:"path,omitempty"`
	Value     any    `json:"value"`
}

// DeleteSurface removes a surface and its tree.
type DeleteSurface struct {
	SurfaceID string `json:"surfaceId"`
}

// CallRendererFunction asks the renderer to run a registered function.
type CallRendererFunction struct {
	SurfaceID      string         `json:"surfaceId,omitempty"`
	FunctionCallID string         `json:"functionCallId"`
	CallFunction   map[string]any `json:"callFunction"`
}

// AgentFunctionResponse completes a renderer-initiated function call.
type AgentFunctionResponse struct {
	FunctionCallID string         `json:"functionCallId"`
	Value          any            `json:"value,omitempty"`
	Error          map[string]any `json:"error,omitempty"`
}

// Validate reports whether an envelope names exactly one payload and speaks a
// supported revision.
func (m Message) Validate() error {
	if m.Version != "" && m.Version != Version && m.Version != "v"+Version {
		return fmt.Errorf("a2ui: unsupported version %q", m.Version)
	}
	n := 0
	for _, set := range []bool{
		m.CreateSurface != nil, m.UpdateComponents != nil,
		m.UpdateDataModel != nil, m.DeleteSurface != nil,
		m.CallRendererFunction != nil, m.AgentFunctionResponse != nil,
	} {
		if set {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("a2ui: message must carry exactly one payload, got %d", n)
	}
	return nil
}

// PayloadName names the single message kind, for errors and routing.
func (m Message) PayloadName() string {
	switch {
	case m.CreateSurface != nil:
		return "createSurface"
	case m.UpdateComponents != nil:
		return "updateComponents"
	case m.UpdateDataModel != nil:
		return "updateDataModel"
	case m.DeleteSurface != nil:
		return "deleteSurface"
	case m.CallRendererFunction != nil:
		return "callRendererFunction"
	case m.AgentFunctionResponse != nil:
		return "agentFunctionResponse"
	}
	return ""
}

// Component is one entry of a surface's flat component list. Properties the
// protocol does not name itself, and the @-prefixed envelope keys of v1.0,
// are kept in Props.
type Component struct {
	ID            string         `json:"id"`
	Type          string         `json:"component"`
	Child         string         `json:"child,omitempty"`
	Children      []string       `json:"children,omitempty"`
	CatalogID     string         `json:"@catalogId,omitempty"`
	Accessibility map[string]any `json:"@accessibility,omitempty"`
	Metadata      map[string]any `json:"@metadata,omitempty"`
	Checks        []Check        `json:"@checks,omitempty"`
	Props         map[string]any `json:"-"`
}

// Check is one component-level validation rule.
type Check struct {
	Message   string `json:"message,omitempty"`
	Condition any    `json:"condition,omitempty"`
}

// reservedComponentKeys are named fields of Component, not Props.
var reservedComponentKeys = map[string]bool{
	"id": true, "component": true, "child": true, "children": true,
	"@catalogId": true, "@accessibility": true, "@metadata": true, "@checks": true,
}

// UnmarshalJSON separates the named fields from catalog properties.
func (c *Component) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Props = map[string]any{}
	for key, value := range raw {
		if reservedComponentKeys[key] {
			continue
		}
		var v any
		if err := json.Unmarshal(value, &v); err != nil {
			return fmt.Errorf("a2ui: component property %q: %w", key, err)
		}
		c.Props[key] = v
	}
	if err := decode(raw, "id", &c.ID); err != nil {
		return err
	}
	if err := decode(raw, "component", &c.Type); err != nil {
		return err
	}
	if err := decode(raw, "child", &c.Child); err != nil {
		return err
	}
	if err := decode(raw, "children", &c.Children); err != nil {
		return err
	}
	if err := decode(raw, "@catalogId", &c.CatalogID); err != nil {
		return err
	}
	if err := decode(raw, "@accessibility", &c.Accessibility); err != nil {
		return err
	}
	if err := decode(raw, "@metadata", &c.Metadata); err != nil {
		return err
	}
	if err := decode(raw, "@checks", &c.Checks); err != nil {
		return err
	}
	return nil
}

// MarshalJSON writes the named fields back with the catalog properties.
func (c Component) MarshalJSON() ([]byte, error) {
	out := map[string]any{"id": c.ID, "component": c.Type}
	for k, v := range c.Props {
		if reservedComponentKeys[k] {
			continue
		}
		out[k] = v
	}
	if c.Child != "" {
		out["child"] = c.Child
	}
	if len(c.Children) > 0 {
		out["children"] = c.Children
	}
	if c.CatalogID != "" {
		out["@catalogId"] = c.CatalogID
	}
	if len(c.Accessibility) > 0 {
		out["@accessibility"] = c.Accessibility
	}
	if len(c.Metadata) > 0 {
		out["@metadata"] = c.Metadata
	}
	if len(c.Checks) > 0 {
		out["@checks"] = c.Checks
	}
	return json.Marshal(out)
}

// decode unmarshals one raw field when present.
func decode(raw map[string]json.RawMessage, key string, dst any) error {
	value, ok := raw[key]
	if !ok {
		return nil
	}
	if err := json.Unmarshal(value, dst); err != nil {
		return fmt.Errorf("a2ui: field %q: %w", key, err)
	}
	return nil
}

// Prop returns one catalog property.
func (c Component) Prop(name string) any { return c.Props[name] }

// PropString returns a string property, or "".
func (c Component) PropString(name string) string {
	s, _ := c.Props[name].(string)
	return s
}

// PropFloat returns a numeric property, or def.
func (c Component) PropFloat(name string, def float64) float64 {
	switch v := c.Props[name].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f
		}
	}
	return def
}

// Weight is a layout weight, as a child of a Row or Column may set.
func (c Component) Weight() float32 {
	return float32(c.PropFloat("weight", 0))
}

// PropBool returns a boolean property, or def.
func (c Component) PropBool(name string, def bool) bool {
	if v, ok := c.Props[name].(bool); ok {
		return v
	}
	return def
}
