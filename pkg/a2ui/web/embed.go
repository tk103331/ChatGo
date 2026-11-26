package web

import _ "embed"

// Built-in web renderer bundles. All of them speak the same host bridge:
// GET /pending receives envelopes and POST /rpc returns actions, size changes,
// renderer messages and errors.
const (
	// BasicBundleID is the vanilla JavaScript renderer shipped with ChatGo.
	BasicBundleID = "basic"
	// ReactBundleID is the official @a2ui/react renderer.
	ReactBundleID = "react"
	// CopilotBundleID is the @copilotkit/a2ui-renderer renderer.
	CopilotBundleID = "copilot"
)

//go:embed bundle.js
var basicBundle []byte

//go:embed bundle.react.js
var reactBundle []byte

//go:embed bundle.copilot.js
var copilotBundle []byte

// BuiltinBundles returns the bundles ChatGo embeds. The map is fresh each
// call, so a caller may add or replace entries without affecting others.
func BuiltinBundles() map[string]*Bundle {
	return map[string]*Bundle{
		BasicBundleID:   {ID: BasicBundleID, JS: basicBundle},
		ReactBundleID:   {ID: ReactBundleID, JS: reactBundle},
		CopilotBundleID: {ID: CopilotBundleID, JS: copilotBundle},
	}
}
