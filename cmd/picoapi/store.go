package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

// Where a provider puts its key.
const (
	keyNone   = ""
	keyHeader = "header" // in the header keyName
	keyBearer = "bearer" // in "Authorization: Bearer <key>"
	keyQuery  = "query"  // in the query parameter keyName
)

// Provider is one service with one key: two keys for the same service are two providers.
type Provider struct {
	Name      string      `json:"name"`
	URL       string      `json:"url"`
	KeyIn     string      `json:"keyIn,omitempty"`
	KeyName   string      `json:"keyName,omitempty"`
	Headers   []Header    `json:"headers,omitempty"`
	Endpoints []*Endpoint `json:"endpoints,omitempty"`
}

// Header is a fixed header sent with every request of a provider.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Endpoint is one request of a provider: Request is the method and the path, as in
// "GET /forecast?city={city}". Values are the last values typed for each variable,
// the most recent first.
type Endpoint struct {
	Name    string              `json:"name"`
	Request string              `json:"request"`
	Body    string              `json:"body,omitempty"`
	Values  map[string][]string `json:"values,omitempty"`
}

// How many values of each variable are remembered.
const maxValues = 10

// remember puts value first among the values of the variable, without repeating it.
func (e *Endpoint) remember(name, value string) {
	if value == "" {
		return
	}
	if e.Values == nil {
		e.Values = map[string][]string{}
	}
	list := []string{value}
	for _, v := range e.Values[name] {
		if v != value && len(list) < maxValues {
			list = append(list, v)
		}
	}
	e.Values[name] = list
}

// Store is everything Pico API keeps, except the keys: those are in the system keychain.
type Store struct {
	Providers []*Provider `json:"providers"`
	SaveDir   string      `json:"saveDir,omitempty"` // folder of the last saved response

	path string
}

// storePath is the file in the standard folder for app settings
// (~/Library/Application Support on Mac, %AppData% on Windows).
func storePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "picoapi", "providers.json"), nil
}

// loadStore reads the file; a file that does not exist yet is an empty store.
func loadStore(path string) (*Store, error) {
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	return s, nil
}

// save writes the file, readable only by the user.
func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) find(name string) *Provider {
	for _, p := range s.Providers {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// ---- Keys in the system keychain ----

const keyringService = "picoapi"

func getKey(provider string) (string, error) {
	k, err := keyring.Get(keyringService, provider)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return k, err
}

func setKey(provider, key string) error {
	return keyring.Set(keyringService, provider, key)
}

func deleteKey(provider string) error {
	err := keyring.Delete(keyringService, provider)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
