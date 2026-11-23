package relay

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Desktop is one machine running ChatGo.
type Desktop struct {
	ID         string
	Name       string
	Version    string
	SecretHash []byte `json:"-"`
	LastSeen   time.Time
}

// Device is one phone paired with a desktop.
type Device struct {
	ID        string
	DesktopID string
	Name      string
	Platform  string
	TokenHash []byte `json:"-"`
	CreatedAt time.Time
	LastSeen  time.Time
	Revoked   bool
	// Disabled keeps the pairing while the relay refuses the device token.
	Disabled     bool
	PushProvider string
	PushToken    string `json:"-"`
	PushEnv      string
}

// PushTarget is one device's push registration.
type PushTarget struct {
	DeviceID string
	Provider string
	Token    string
	Env      string
}

// Store persists desktops, devices and push tokens in SQLite.
type Store struct {
	db *sql.DB
}

// OpenStore opens the relay database, creating its schema and parent
// directory. An empty path keeps everything in memory for tests.
func OpenStore(path string) (*Store, error) {
	dsn := "file:chatgo-relay-" + NewID("") + "?mode=memory&cache=shared"
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		dsn = path
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// modernc's driver is safe for concurrent use, but one connection keeps
	// the in-memory schema visible to every caller.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS desktops (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL DEFAULT '',
  version TEXT NOT NULL DEFAULT '',
  secret_hash BLOB NOT NULL,
  created_at INTEGER NOT NULL,
  last_seen INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS devices (
  id TEXT PRIMARY KEY,
  desktop_id TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  platform TEXT NOT NULL DEFAULT '',
  token_hash BLOB NOT NULL,
  created_at INTEGER NOT NULL,
  last_seen INTEGER NOT NULL,
  revoked INTEGER NOT NULL DEFAULT 0,
  push_provider TEXT NOT NULL DEFAULT '',
  push_token TEXT NOT NULL DEFAULT '',
  push_env TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS devices_desktop ON devices(desktop_id);
`)
	if err != nil {
		return err
	}
	// Databases from before the enabled switch predate this column.
	if _, err := s.db.Exec(`ALTER TABLE devices ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column") {
		return err
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// RegisterDesktop creates the desktop on first sight and verifies its secret
// afterwards. Sighting an id for the first time is trust-on-first-use; the id
// is a 128-bit random value the desktop generates and never advertises.
func (s *Store) RegisterDesktop(d Desktop, secretHash []byte) (*Desktop, error) {
	var stored []byte
	var name, version string
	var lastSeen int64
	err := s.db.QueryRow(`SELECT secret_hash, name, version, last_seen FROM desktops WHERE id = ?`, d.ID).
		Scan(&stored, &name, &version, &lastSeen)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		now := time.Now().Unix()
		_, err = s.db.Exec(`INSERT INTO desktops (id, name, version, secret_hash, created_at, last_seen)
VALUES (?, ?, ?, ?, ?, ?)`, d.ID, d.Name, d.Version, secretHash, now, now)
		if err != nil {
			return nil, err
		}
		return &Desktop{ID: d.ID, Name: d.Name, Version: d.Version, SecretHash: secretHash, LastSeen: time.Unix(now, 0)}, nil
	case err != nil:
		return nil, err
	}
	if subtle.ConstantTimeCompare(stored, secretHash) != 1 {
		return nil, ErrUnauthorized
	}
	now := time.Now().Unix()
	_, err = s.db.Exec(`UPDATE desktops SET name = ?, version = ?, last_seen = ? WHERE id = ?`,
		d.Name, d.Version, now, d.ID)
	if err != nil {
		return nil, err
	}
	return &Desktop{ID: d.ID, Name: d.Name, Version: d.Version, SecretHash: stored, LastSeen: time.Unix(now, 0)}, nil
}

// TouchDesktop records a tunnel connection.
func (s *Store) TouchDesktop(id string) {
	_, _ = s.db.Exec(`UPDATE desktops SET last_seen = ? WHERE id = ?`, time.Now().Unix(), id)
}

// AddDevice records a newly paired phone.
func (s *Store) AddDevice(d Device) error {
	_, err := s.db.Exec(`INSERT INTO devices
(id, desktop_id, name, platform, token_hash, created_at, last_seen, revoked, disabled)
VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0)`,
		d.ID, d.DesktopID, d.Name, d.Platform, d.TokenHash, d.CreatedAt.Unix(), d.LastSeen.Unix())
	return err
}

// DeviceByToken finds the device a bearer token belongs to, scoped to one
// desktop. It touches the device's last-seen time on success.
func (s *Store) DeviceByToken(desktopID string, tokenHash []byte) (*Device, error) {
	rows, err := s.db.Query(`SELECT id, desktop_id, name, platform, token_hash, created_at, last_seen, revoked, disabled
FROM devices WHERE desktop_id = ? AND revoked = 0`, desktopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found *Device
	for rows.Next() {
		var d Device
		var created, seen int64
		if err := rows.Scan(&d.ID, &d.DesktopID, &d.Name, &d.Platform, &d.TokenHash, &created, &seen, &d.Revoked, &d.Disabled); err != nil {
			return nil, err
		}
		if subtle.ConstantTimeCompare(d.TokenHash, tokenHash) == 1 {
			d.CreatedAt = time.Unix(created, 0)
			d.LastSeen = time.Unix(seen, 0)
			copyD := d
			found = &copyD
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if found == nil {
		return nil, ErrUnauthorized
	}
	// A disabled device keeps its pairing but the relay refuses its token,
	// so the phone can tell the user the desktop paused it rather than
	// treating the phone as dropped and demanding a new pair.
	if found.Disabled {
		return nil, ErrDeviceDisabled
	}
	_, _ = s.db.Exec(`UPDATE devices SET last_seen = ? WHERE id = ?`, time.Now().Unix(), found.ID)
	return found, nil
}

// ListDevices lists one desktop's paired phones.
func (s *Store) ListDevices(desktopID string) ([]Device, error) {
	rows, err := s.db.Query(`SELECT id, desktop_id, name, platform, created_at, last_seen, revoked, disabled,
push_provider, push_token, push_env FROM devices WHERE desktop_id = ? ORDER BY created_at DESC`, desktopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		var created, seen int64
		if err := rows.Scan(&d.ID, &d.DesktopID, &d.Name, &d.Platform, &created, &seen, &d.Revoked, &d.Disabled,
			&d.PushProvider, &d.PushToken, &d.PushEnv); err != nil {
			return nil, err
		}
		d.CreatedAt = time.Unix(created, 0)
		d.LastSeen = time.Unix(seen, 0)
		out = append(out, d)
	}
	return out, rows.Err()
}

// RevokeDevice marks a device revoked. Unknown ids are not an error, so a
// desktop can retry a revocation after a relay restart.
func (s *Store) RevokeDevice(desktopID, deviceID string) error {
	_, err := s.db.Exec(`UPDATE devices SET revoked = 1 WHERE id = ? AND desktop_id = ?`, deviceID, desktopID)
	return err
}

// SetDeviceDisabled keeps a device paired while its token is refused.
func (s *Store) SetDeviceDisabled(desktopID, deviceID string, disabled bool) error {
	_, err := s.db.Exec(`UPDATE devices SET disabled = ? WHERE id = ? AND desktop_id = ?`,
		disabled, deviceID, desktopID)
	return err
}

// SetPushToken records or clears one device's push registration.
func (s *Store) SetPushToken(deviceID, desktopID, provider, token, env string) error {
	_, err := s.db.Exec(`UPDATE devices SET push_provider = ?, push_token = ?, push_env = ?
WHERE id = ? AND desktop_id = ?`, provider, token, env, deviceID, desktopID)
	return err
}

// PushTargets lists the push registrations of a desktop's live devices.
func (s *Store) PushTargets(desktopID string) ([]PushTarget, error) {
	rows, err := s.db.Query(`SELECT id, push_provider, push_token, push_env FROM devices
WHERE desktop_id = ? AND revoked = 0 AND disabled = 0 AND push_token <> ''`, desktopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PushTarget
	for rows.Next() {
		var t PushTarget
		if err := rows.Scan(&t.DeviceID, &t.Provider, &t.Token, &t.Env); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
