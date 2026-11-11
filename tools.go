//go:build tools

// The mygo CLI is built from this module by build.sh and the release workflow.
// Importing it here keeps its dependencies in go.mod and go.sum, so those
// builds work after `go mod tidy` without adding the CLI to the app itself.
package main

import _ "github.com/egoist/mygo/cmd/mygo"
