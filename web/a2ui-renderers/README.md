# A2UI web renderer bundles

Builds the optional React renderers ChatGo embeds for the A2UI web surface.

| Entry | Output | Bundle ID |
|---|---|---|
| `src/react.tsx` | `pkg/a2ui/web/bundle.react.js` | `react` |
| `src/copilot.tsx` | `pkg/a2ui/web/bundle.copilot.js` | `copilot` |

```sh
npm install
npm run build
```

The Go binary embeds the generated files, so a normal `go build` does not need
Node. Rebuild and commit the outputs when either entry or its dependencies
change.

## Host contract

Each page is loaded from `a2ui-web://<token>/` under a strict CSP and must be a
single self-contained script. The bundle:

- polls `GET /pending` for one A2UI envelope at a time;
- exposes `window.a2ui.apply(message | messages)` for direct injection;
- posts `POST /rpc` with `{type: "action", action, dataModel}` on user actions;
- posts `{type: "resize", width, height}` when its height changes;
- forwards renderer messages as `{type: "rendererMessage", rendererMessage}`;
- reports failures as `{type: "error", message}`.

`src/bridge.ts` implements that contract; the two entries only mount their
renderer and feed envelopes into it.