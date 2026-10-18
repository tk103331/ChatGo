package webapp

import "encoding/json"

// decodeJSON parses a webapp.json manifest.
func decodeJSON(data []byte, m *Manifest) error {
	return json.Unmarshal(data, m)
}
