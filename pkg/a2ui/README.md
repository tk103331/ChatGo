# a2ui

A2UI 1.0 for Go and MyGo. The package is split so the protocol can be reused
without a UI dependency, and an app can render a surface natively, in a Web
View, or choose per surface.

```text
a2ui/          protocol: envelopes, surfaces, data model, bindings, actions
a2ui/native/   MyGo native renderer
a2ui/web/      Web View renderer and bridge
a2ui/hybrid/   per-surface routing between the two
a2ui/render/   the Renderer interface both implement
```

Nothing here imports `chatgo/internal/*`; the only UI dependency is
`github.com/egoist/mygo/ui`.

## Native

```go
r := native.New(native.Options{
    Theme: native.DefaultTheme(),
    OnAction: func(a a2ui.Action) {
        // Send a.Envelope() back to the agent.
    },
    Invalidate: func() { win.Invalidate() },
})
errs := r.Apply(msgs...)

// In the window's view:
r.View(c, "contact_form")
```

Supported natively: `Text`, `Image`, `Row`, `Column`, `List`, `Card`,
`Divider`, `Button`, `TextField`, `CheckBox`, `Slider`, `Select`,
`ChoicePicker`. Unknown components draw a labelled placeholder instead of
disappearing.

## Web

The web renderer keeps the view model in the page and bridges actions and
size changes over a custom scheme.

```go
mgr := web.NewManager(nil) // nil uses the embedded fallback bundle
if err := mygo.Protocol.Handle(web.Scheme, mgr); err != nil {
    log.Fatal(err)
}
// Create the window after registering the scheme.

r := web.New(win, mgr, web.Options{
    ScrollToContainer: true, // inline in a native scroll
    Invalidate:        func() { win.Invalidate() },
    OnAction: func(a a2ui.Action, data map[string]any) {
        // ...
    },
})
r.Apply(msgs...)
```

`web.NewManager(customBundle)` accepts any JavaScript bundle that implements
the same `apply(message)` and `POST /rpc` contract. The embedded bundle is a
small, dependency-free renderer for the basic components; a full catalog such
as CopilotKit's `standardUI` can be shipped as a custom bundle instead.

The page receives `createSurface`, `updateComponents`, `updateDataModel` and
`deleteSurface`, resolves `@path` bindings, updates its data model in place,
and posts `action` / `resize` / `error` messages back. The embedded bundle
covers the full A2UI 1.0 Basic Catalog (18 components), including Tabs, Modal,
Video, AudioPlayer and DateTimeInput, plus repeating `children` templates and
the basic catalog functions.

## Hybrid

```go
router := hybrid.New(
    native.New(native.Options{/* ... */}),
    web.New(win, mgr, web.Options{/* ... */}),
)
router.Apply(msgs...)
router.View(c, surfaceID)
```

A surface whose `catalogId` (and every component's `@catalogId`) is native goes
to the native renderer. A custom or rich catalog goes to the Web View. Either
renderer may be nil, in which case a surface that needs it is an error.

## Limits

- A2UI 1.0 UI envelopes and renderer functions. `callRendererFunction` runs
  a registered function and reports `rendererFunctionResponse`;
  `callAgentFunction` is available on both renderers and completes when an
  `agentFunctionResponse` arrives. Function results still travel through the
  host's transport, which ChatGo sends as the next turn, not into a live run.
- Whole-surface routing. Mixing native and web components inside one surface
  needs fragment surfaces and is left for a later pass.
- The native renderer does not implement charts, maps, video, KaTeX or raw
  HTML; those belong to the web renderer.
- Two-way bindings write into the protocol store; an agent update replaces the
  local field on the next frame.
- `native.Renderer.SetTheme` and `web.Renderer.SetTheme` change the palette;
  MyGo redraws the next frame.
- The embedded web bundle is a reference renderer, not the CopilotKit
  `standardUI` catalog. Supply that bundle explicitly if you need its 69
  components.

## Transport

This package does not own AG-UI, A2A or MCP. A host decodes the A2UI payloads
(`CUSTOM name=a2ui` over AG-UI, `application/a2ui+json` DataPart over A2A) into
`[]a2ui.Message`, calls `Apply`, and sends `a2ui.Action.Envelope()` back on the
transport's return channel.