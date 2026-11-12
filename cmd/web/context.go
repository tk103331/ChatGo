package main

import (
	"strings"
)

// toggleContext opens the composer's context picker, loading the current
// session's workspace files the first time it is shown.
func (a *app) toggleContext() {
	id := a.activeID()
	if id == "" {
		return
	}
	if a.contextOpen {
		a.contextOpen = false
		return
	}
	a.contextOpen = true
	a.contextQuery = ""
	if a.contextFor == id && (a.contextLoading || len(a.contextFiles) > 0 || a.contextErr != "") {
		return
	}
	a.contextFor = id
	a.contextFiles = nil
	a.contextErr = ""
	a.contextLoading = true
	a.win.Update(func() {})
	go func() {
		var resp contextResponse
		err := apiGet(sessionPath(id)+"/context", &resp)
		a.win.Update(func() {
			if a.contextFor != id {
				return
			}
			a.contextLoading = false
			if err != nil {
				a.contextErr = err.Error()
				return
			}
			a.contextFiles = resp.Items
		})
	}()
}

func (a *app) insertContext(item contextItem) {
	if strings.TrimSpace(item.Path) == "" {
		return
	}
	if a.draft != "" && !strings.HasSuffix(a.draft, " ") {
		a.draft += " "
	}
	a.draft += "@" + item.Path + " "
	a.contextOpen = false
	a.win.Update(func() {})
}

func (a *app) visibleContextFiles() []contextItem {
	q := strings.ToLower(strings.TrimSpace(a.contextQuery))
	if q == "" {
		return a.contextFiles
	}
	out := make([]contextItem, 0, len(a.contextFiles))
	for _, item := range a.contextFiles {
		if strings.Contains(strings.ToLower(item.Label), q) {
			out = append(out, item)
		}
	}
	return out
}
