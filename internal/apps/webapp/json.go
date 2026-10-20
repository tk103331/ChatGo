package webapp

import "encoding/json"

// decodeJSON parses a webapp.json manifest.
func decodeJSON(data []byte, m *Manifest) error {
	return json.Unmarshal(data, m)
}

// callScript builds the page script invoking one registered tool and posting
// its result back to the bridge: window.chatgo.handle(name, args).
func callScript(id, name string, args map[string]any) (string, error) {
	nameJS, err := jsonFor(name)
	if err != nil {
		return "", err
	}
	argsJS, err := jsonFor(args)
	if err != nil {
		return "", err
	}
	idJS, err := jsonFor(id)
	if err != nil {
		return "", err
	}
	post := func(field, value string) string {
		return `fetch("/__chatgo/result",{method:"POST",headers:{"Content-Type":"application/json"},` +
			`body:JSON.stringify({id:` + string(idJS) + `,` + field + `:` + value + `})})`
	}
	return `(function(){try{var p=window.chatgo.handle(` + string(nameJS) + `,` + string(argsJS) + `);` +
		`Promise.resolve(p).then(function(r){` + post("result", "r") + `},` +
		`function(e){` + post("error", "String(e)") + `});}catch(e){` + post("error", "String(e)") + `;}` +
		`return "pending"})()`, nil
}
