// Package config holds what the CLI remembers between runs: the config file, credentials per
// profile (keychain or a 0600 file), and the directory-to-app binding in .whisk/app.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/whisk-run/cli/internal/safefile"
)

// DefaultAPI is the production control plane.
const DefaultAPI = "https://api.whisk.run"

// Config is $XDG_CONFIG_HOME/whisk/config.json (CLI.md §7). Dashboard is the origin the CLI
// prints links to; empty derives it from the API origin (api.<domain> → <domain>).
type Config struct {
	API       string `json:"api"`
	Profile   string `json:"profile"`
	Telemetry bool   `json:"telemetry"`
	Dashboard string `json:"dashboard,omitempty"`
}

// Dir returns the configuration directory for the given environment lookup.
func Dir(getenv func(string) string) string {
	if x := getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "whisk")
	}
	if d, err := os.UserConfigDir(); err == nil && d != "" {
		return filepath.Join(d, "whisk")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "whisk")
}

// Load reads config.json, filling defaults; a missing file is the defaults.
func Load(dir string) (Config, error) {
	cfg := Config{API: DefaultAPI, Profile: "default"}
	src, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(src, &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %v", filepath.Join(dir, "config.json"), err)
	}
	if cfg.API == "" {
		cfg.API = DefaultAPI
	}
	if cfg.Profile == "" {
		cfg.Profile = "default"
	}
	return cfg, nil
}

// Save writes config.json.
func Save(dir string, cfg Config) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	out, _ := json.MarshalIndent(cfg, "", "  ")
	return safefile.WriteFile(filepath.Join(dir, "config.json"), append(out, '\n'), 0o600)
}

// Credential is one profile's token and what it is for.
type Credential struct {
	Token     string    `json:"token"`
	API       string    `json:"api"`
	Org       string    `json:"org,omitempty"`
	OrgID     string    `json:"org_id,omitempty"`
	User      string    `json:"user,omitempty"`
	Scopes    []string  `json:"scopes,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	// PrivateKey is this computer's Ed25519 private key (its 32-byte seed, standard base64) when
	// the token is bound to it, as every token from whisk login is (CLI.md §2). Requests with the
	// token are signed with it; the token alone is useless elsewhere.
	PrivateKey string `json:"private_key,omitempty"`
	// PendingCode is set only on a login waiting for --resume: the hex SHA-256 of its device code,
	// kept with the key that login made until the token is collected.
	PendingCode string `json:"pending_code,omitempty"`
}

// PendingProfile is where a --no-wait login keeps its key until --resume collects the token,
// beside the profile's credential rather than over it.
func PendingProfile(profile string) string { return profile + ":pending" }

// Store keeps credentials per profile.
type Store interface {
	Get(profile string) (Credential, bool, error)
	Set(profile string, c Credential) error
	Delete(profile string) error
	// Where says where credentials live, for whoami and for humans.
	Where() string
}

const keyringService = "whisk"

// OpenStore returns the OS keychain when one answers, else the credentials file.
func OpenStore(dir string) Store {
	if _, err := keyring.Get(keyringService, "whisk-probe"); err == nil || errors.Is(err, keyring.ErrNotFound) {
		return keyringStore{}
	}
	return fileStore{path: filepath.Join(dir, "credentials.json")}
}

type keyringStore struct{}

func (keyringStore) Where() string { return "os keychain" }

func (keyringStore) Get(profile string) (Credential, bool, error) {
	raw, err := keyring.Get(keyringService, profile)
	if errors.Is(err, keyring.ErrNotFound) {
		return Credential{}, false, nil
	}
	if err != nil {
		return Credential{}, false, err
	}
	var c Credential
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return Credential{}, false, err
	}
	return c, true, nil
}

func (keyringStore) Set(profile string, c Credential) error {
	raw, _ := json.Marshal(c)
	return keyring.Set(keyringService, profile, string(raw))
}

func (keyringStore) Delete(profile string) error {
	err := keyring.Delete(keyringService, profile)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// MemoryStore keeps credentials in memory only, for a program driving the CLI as a library
// (the harness): nothing reaches the keychain or the disk, and nothing outlives the process.
type MemoryStore struct {
	mu sync.Mutex
	m  map[string]Credential
}

// NewMemoryStore holds one profile's credential.
func NewMemoryStore(profile string, c Credential) *MemoryStore {
	return &MemoryStore{m: map[string]Credential{profile: c}}
}

func (s *MemoryStore) Where() string { return "memory" }

func (s *MemoryStore) Get(profile string) (Credential, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.m[profile]
	return c, ok, nil
}

func (s *MemoryStore) Set(profile string, c Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[profile] = c
	return nil
}

func (s *MemoryStore) Delete(profile string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, profile)
	return nil
}

type fileStore struct{ path string }

type credentialsFile struct {
	Profiles map[string]Credential `json:"profiles"`
}

func (f fileStore) Where() string { return f.path }

func (f fileStore) read() (credentialsFile, error) {
	var cf credentialsFile
	src, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return credentialsFile{Profiles: map[string]Credential{}}, nil
	}
	if err != nil {
		return cf, err
	}
	if err := json.Unmarshal(src, &cf); err != nil {
		return cf, fmt.Errorf("%s: %v", f.path, err)
	}
	if cf.Profiles == nil {
		cf.Profiles = map[string]Credential{}
	}
	return cf, nil
}

func (f fileStore) write(cf credentialsFile) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	out, _ := json.MarshalIndent(cf, "", "  ")
	// Through a temporary file: never half written, and 0600 even when an older file was not.
	return safefile.WriteFile(f.path, append(out, '\n'), 0o600)
}

func (f fileStore) Get(profile string) (Credential, bool, error) {
	cf, err := f.read()
	if err != nil {
		return Credential{}, false, err
	}
	c, ok := cf.Profiles[profile]
	return c, ok, nil
}

func (f fileStore) Set(profile string, c Credential) error {
	cf, err := f.read()
	if err != nil {
		return err
	}
	cf.Profiles[profile] = c
	return f.write(cf)
}

func (f fileStore) Delete(profile string) error {
	cf, err := f.read()
	if err != nil {
		return err
	}
	delete(cf.Profiles, profile)
	return f.write(cf)
}

// Binding is .whisk/app.json: which org and app a directory belongs to, and which API.
type Binding struct {
	Org string `json:"org"`
	App string `json:"app"`
	API string `json:"api"`
}

// BindingPath is where the binding lives relative to the app directory.
const BindingPath = ".whisk/app.json"

// LoadBinding reads .whisk/app.json from dir. ok is false when the directory is not bound.
func LoadBinding(dir string) (Binding, bool, error) {
	src, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(BindingPath)))
	if errors.Is(err, os.ErrNotExist) {
		return Binding{}, false, nil
	}
	if err != nil {
		return Binding{}, false, err
	}
	var b Binding
	if err := json.Unmarshal(src, &b); err != nil {
		return Binding{}, false, fmt.Errorf("%s: %v", BindingPath, err)
	}
	if b.Org == "" || b.App == "" {
		return Binding{}, false, fmt.Errorf("%s: org and app are required", BindingPath)
	}
	return b, true, nil
}

// SaveBinding writes .whisk/app.json.
func SaveBinding(dir string, b Binding) error {
	out, _ := json.MarshalIndent(b, "", "  ")
	return safefile.Write(dir, BindingPath, append(out, '\n'), 0o644)
}

// ParseRef splits "org/app" into its parts.
func ParseRef(ref string) (org, app string, err error) {
	org, app, ok := strings.Cut(ref, "/")
	if !ok || org == "" || app == "" || strings.Contains(app, "/") {
		return "", "", fmt.Errorf("%q is not <org>/<app>", ref)
	}
	return org, app, nil
}
