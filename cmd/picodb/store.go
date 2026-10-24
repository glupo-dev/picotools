package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

// Connection is a saved database: only its name is in the file, the connection string
// (which holds the password) is in the system keychain.
type Connection struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"` // sqlserver or postgres; empty is sqlserver

	Favourites map[string][]string `json:"favourites,omitempty"` // database → favourite tables (schema.name)
}

// Store is everything Pico DB keeps, except the connection strings.
type Store struct {
	Connections []*Connection `json:"connections"`
	SaveDir     string        `json:"saveDir,omitempty"` // folder of the last saved file

	path string
}

// storePath is the file in the standard folder for app settings
// (~/Library/Application Support on Mac, %AppData% on Windows).
func storePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "picodb", "connections.json"), nil
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

func (s *Store) find(name string) *Connection {
	for _, c := range s.Connections {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// ---- Connection strings in the system keychain ----

const keyringService = "picodb"

func getConnString(name string) (string, error) {
	k, err := keyring.Get(keyringService, name)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return k, err
}

func setConnString(name, value string) error {
	return keyring.Set(keyringService, name, value)
}

func deleteConnString(name string) error {
	err := keyring.Delete(keyringService, name)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
