package webui

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxAttachmentBytes bounds one browser upload. A dropped file is read whole
// into memory by the wasm client, so the limit keeps both sides small.
const maxAttachmentBytes = 8 << 20

// attachmentRef names one uploaded file a message carries.
type attachmentRef struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// uploadRoot is where browser uploads live, outside any user workspace.
func uploadRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".chatgo", "uploads"), nil
}

func attachmentDir(convID string) (string, error) {
	root, err := uploadRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, convID), nil
}

// handleAttachment stores one dropped file and returns its path. The browser
// sends base64 because the wasm client has the bytes, not a multipart form.
func (s *Server) handleAttachment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, errors.New("invalid session id"))
		return
	}
	var body struct {
		Name string `json:"name"`
		Data string `json:"data"`
	}
	if err := readJSONLimit(r, &body, 12<<20); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	name := safeAttachmentName(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, errors.New("attachment name is empty"))
		return
	}
	data, err := base64.StdEncoding.DecodeString(body.Data)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("attachment data is not base64"))
		return
	}
	if len(data) > maxAttachmentBytes {
		writeError(w, http.StatusRequestEntityTooLarge, errors.New("attachment is too large"))
		return
	}
	if _, ok := s.loadSession(w, id); !ok {
		return
	}
	dir, err := attachmentDir(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	path := filepath.Join(dir, newID()+"-"+name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"attachment": attachmentRef{Name: name, Path: path},
	})
}

// safeAttachmentName keeps a dropped file's base name and drops control
// characters, so a path never escapes the upload directory.
func safeAttachmentName(name string) string {
	name = strings.TrimSpace(name)
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	if name == "." || name == ".." {
		return ""
	}
	return name
}

// embedAttachments appends the text of a message's uploads so the model sees
// them even when no file tool is enabled. Binary uploads stay on disk and are
// named in the message instead.
func embedAttachments(text, convID string, refs []attachmentRef) string {
	if len(refs) == 0 {
		return text
	}
	dir, err := attachmentDir(convID)
	if err != nil {
		return text
	}
	var b strings.Builder
	b.WriteString(text)
	for _, ref := range refs {
		path, err := filepath.Abs(ref.Path)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		name := strings.TrimSpace(ref.Name)
		if name == "" {
			name = filepath.Base(path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if len(data) > 256<<10 {
			b.WriteString(fmt.Sprintf("\n\n[附件 %s：%d 字节，路径 %s]", name, len(data), path))
			continue
		}
		if !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
			b.WriteString(fmt.Sprintf("\n\n[附件 %s：二进制文件，路径 %s]", name, path))
			continue
		}
		b.WriteString(fmt.Sprintf("\n\n[附件 %s]\n%s", name, data))
	}
	return b.String()
}
