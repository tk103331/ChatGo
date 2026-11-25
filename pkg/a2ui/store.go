package a2ui

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// Surface is an immutable snapshot of one rendered surface.
type Surface struct {
	ID            string
	CatalogID     string
	SendDataModel bool
	Components    map[string]Component
	Data          any
	Metadata      map[string]any
}

// Component returns one component by ID.
func (s *Surface) Component(id string) (Component, bool) {
	c, ok := s.Components[id]
	return c, ok
}

// SurfaceVersion is a snapshot and the revision it was taken at.
type SurfaceVersion struct {
	Surface  *Surface
	Revision uint64
}

// Store applies A2UI messages and keeps the current surfaces. It is safe for
// one goroutine to Apply while others take snapshots.
type Store struct {
	mu       sync.RWMutex
	surfaces map[string]*surfaceRecord
}

type surfaceRecord struct {
	surface  Surface
	revision uint64
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{surfaces: map[string]*surfaceRecord{}}
}

// MessageError reports which message failed and why. Applying continues with
// the messages after it, as the A2A binding requires.
type MessageError struct {
	Index int
	Err   error
}

func (e MessageError) Error() string {
	return fmt.Sprintf("a2ui: message %d: %v", e.Index, e.Err)
}

func (e MessageError) Unwrap() error { return e.Err }

// Apply applies messages in order. An empty result means all succeeded.
func (s *Store) Apply(messages ...Message) []error {
	var errs []error
	for i, msg := range messages {
		if err := msg.Validate(); err != nil {
			errs = append(errs, MessageError{Index: i, Err: err})
			continue
		}
		if err := s.applyOne(msg); err != nil {
			errs = append(errs, MessageError{Index: i, Err: err})
		}
	}
	return errs
}

func (s *Store) applyOne(msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case msg.CreateSurface != nil:
		in := msg.CreateSurface
		if in.SurfaceID == "" {
			return fmt.Errorf("createSurface without surfaceId")
		}
		if _, exists := s.surfaces[in.SurfaceID]; exists {
			return fmt.Errorf("surface %q already exists", in.SurfaceID)
		}
		rec := &surfaceRecord{surface: Surface{
			ID: in.SurfaceID, CatalogID: in.CatalogID, SendDataModel: in.SendDataModel,
			Components: map[string]Component{}, Data: map[string]any{},
			Metadata: in.Metadata,
		}, revision: 1}
		if in.DataModel != nil {
			rec.surface.Data = in.DataModel
		}
		for _, c := range in.Components {
			if c.ID == "" {
				return fmt.Errorf("component without id")
			}
			rec.surface.Components[c.ID] = c
		}
		s.surfaces[in.SurfaceID] = rec
		return nil
	case msg.UpdateComponents != nil:
		in := msg.UpdateComponents
		rec := s.surfaces[in.SurfaceID]
		if rec == nil {
			return fmt.Errorf("updateComponents for unknown surface %q", in.SurfaceID)
		}
		for _, c := range in.Components {
			if c.ID == "" {
				return fmt.Errorf("component without id")
			}
			rec.surface.Components[c.ID] = c
		}
		rec.revision++
		return nil
	case msg.UpdateDataModel != nil:
		in := msg.UpdateDataModel
		rec := s.surfaces[in.SurfaceID]
		if rec == nil {
			return fmt.Errorf("updateDataModel for unknown surface %q", in.SurfaceID)
		}
		path := in.Path
		if path == "" {
			path = "/"
		}
		if path == "/" {
			rec.surface.Data = in.Value
		} else {
			root, err := PointerSet(rec.surface.Data, path, in.Value)
			if err != nil {
				return err
			}
			rec.surface.Data = root
		}
		rec.revision++
		return nil
	case msg.DeleteSurface != nil:
		delete(s.surfaces, msg.DeleteSurface.SurfaceID)
		return nil
	default:
		return fmt.Errorf("%s is handled by the renderer, not the store", msg.PayloadName())
	}
}

// Snapshot returns a deep copy of a surface and its revision.
func (s *Store) Snapshot(id string) (SurfaceVersion, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec := s.surfaces[id]
	if rec == nil {
		return SurfaceVersion{}, false
	}
	return SurfaceVersion{Surface: cloneSurface(&rec.surface), Revision: rec.revision}, true
}

// Revision reports a surface's current revision without copying it.
func (s *Store) Revision(id string) (uint64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec := s.surfaces[id]
	if rec == nil {
		return 0, false
	}
	return rec.revision, true
}

// SurfaceIDs lists the live surface identifiers.
func (s *Store) SurfaceIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.surfaces))
	for id := range s.surfaces {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// SetData writes one data-model path, as a two-way bound input does, and
// returns the new revision.
func (s *Store) SetData(surfaceID, path string, value any) (uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.surfaces[surfaceID]
	if rec == nil {
		return 0, false
	}
	if path == "" || path == "/" {
		rec.surface.Data = value
	} else {
		root, err := PointerSet(rec.surface.Data, path, value)
		if err != nil {
			return rec.revision, false
		}
		rec.surface.Data = root
	}
	rec.revision++
	return rec.revision, true
}

// DataModel returns the current data model of a surface.
func (s *Store) DataModel(surfaceID string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec := s.surfaces[surfaceID]
	if rec == nil {
		return nil, false
	}
	return cloneAny(rec.surface.Data), true
}

// cloneSurface deep-copies a surface so a renderer can keep it across frames
// without holding the store lock.
func cloneSurface(in *Surface) *Surface {
	if in == nil {
		return nil
	}
	out := *in
	out.Components = make(map[string]Component, len(in.Components))
	for id, c := range in.Components {
		cc := c
		cc.Children = append([]string(nil), c.Children...)
		cc.Checks = append([]Check(nil), c.Checks...)
		if c.Props != nil {
			cc.Props = cloneAny(c.Props).(map[string]any)
		}
		if c.Accessibility != nil {
			cc.Accessibility = cloneAny(c.Accessibility).(map[string]any)
		}
		if c.Metadata != nil {
			cc.Metadata = cloneAny(c.Metadata).(map[string]any)
		}
		out.Components[id] = cc
	}
	out.Data = cloneAny(in.Data)
	if in.Metadata != nil {
		out.Metadata = cloneAny(in.Metadata).(map[string]any)
	}
	return &out
}

// cloneAny deep-copies a decoded JSON value.
func cloneAny(v any) any {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return v
	}
	return out
}
