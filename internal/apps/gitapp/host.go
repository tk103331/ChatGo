// Package gitapp is the Apps menu's Git client: a repository browser with
// staging, committing, history, branches, stashes and tags, plus the same
// operations exposed to the Agent as tools.
package gitapp

import (
	"chatgo/internal/apps/appkit"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mgit "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/git"
)

// Host is the capability set the client needs from the application shell.
type Host = appkit.Host

// Severity aliases keep call sites short.
const (
	SeverityError   = mcore.SeverityError
	SeveritySuccess = mcore.SeveritySuccess
)

// ChangedFile re-exports the component's file model for the shell.
type ChangedFile = mgit.ChangedFile

// Commit re-exports the component's commit model for the shell.
type Commit = mgit.Commit
