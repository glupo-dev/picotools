package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/argon2"
)

// The vault is one file with every item, encrypted with a key made from the master password,
// as in Pico Auth (cmd/picoauth/vault.go): a small JSON envelope with how the key is made
// (Argon2id with its salt and costs) and the items encrypted with AES-256-GCM. Nothing about
// the items is readable without the password, not even their names.
//
// This first version is local only: no second key, no sync (docs/picopassword.md).

const (
	fileFormat  = "picopassword"
	fileVersion = 1
)

// envelope is the file on disk.
type envelope struct {
	Format  string `json:"format"`  // always "picopassword"
	Version int    `json:"version"` // 1
	KDF     kdf    `json:"kdf"`
	Nonce   []byte `json:"nonce"` // AES-GCM nonce, new at each save
	Data    []byte `json:"data"`  // the encrypted contents
}

// kdf says how the key was made from the password, so the costs can change in later versions.
type kdf struct {
	Name    string `json:"name"` // "argon2id"
	Salt    []byte `json:"salt"`
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"` // KiB
	Threads uint8  `json:"threads"`
}

// contents is what is encrypted.
type contents struct {
	Version int     `json:"version"`
	Items   []*Item `json:"items"`
}

// Item is one saved login.
type Item struct {
	ID        string        `json:"id"` // hidden, to find the item again when it changes (docs 5.2)
	Name      string        `json:"name"`
	User      string        `json:"user"`
	Password  string        `json:"password"`
	Favourite bool          `json:"favourite,omitempty"`
	History   []OldPassword `json:"history,omitempty"` // the most recent first, at most maxHistory
}

// OldPassword is a password replaced by a newer one, with the day it was replaced.
type OldPassword struct {
	Password string    `json:"password"`
	Replaced time.Time `json:"replaced"`
}

const maxHistory = 10

// newID is a random id for a new item.
func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Argon2id costs for new vaults: about a tenth of a second on a laptop.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 4
)

var errWrongPassword = errors.New("wrong password")

// Vault holds the items in memory, with the key to save them again.
type Vault struct {
	Items []*Item

	path     string
	password string
	kdf      kdf
	key      []byte
}

// vaultPath is the file in the standard folder for app settings
// (~/Library/Application Support on Mac, %AppData% on Windows).
func vaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "picopassword", "vault.picopassword"), nil
}

// newVault makes an empty vault protected by password; it is written by save.
func newVault(path, password string) (*Vault, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	k := kdf{Name: "argon2id", Salt: salt, Time: argonTime, Memory: argonMemory, Threads: argonThreads}
	return &Vault{path: path, password: password, kdf: k, key: k.key(password)}, nil
}

func (k kdf) key(password string) []byte {
	return argon2.IDKey([]byte(password), k.Salt, k.Time, k.Memory, k.Threads, 32)
}

// openVault reads and decrypts the file at path; a wrong password gives errWrongPassword.
func openVault(path, password string) (*Vault, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil || env.Format != fileFormat {
		return nil, errors.New("this is not a Pico Password file")
	}
	if env.Version > fileVersion {
		return nil, errors.New("this file was written by a newer Pico Password: update the app")
	}
	if env.KDF.Name != "argon2id" || len(env.KDF.Salt) == 0 || env.KDF.Memory == 0 {
		return nil, errors.New("this Pico Password file is damaged")
	}
	v := &Vault{path: path, password: password, kdf: env.KDF, key: env.KDF.key(password)}
	gcm, err := newGCM(v.key)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, env.Nonce, env.Data, nil)
	if err != nil {
		return nil, errWrongPassword // or the file was changed: GCM cannot tell the two apart
	}
	var c contents
	if err := json.Unmarshal(plain, &c); err != nil {
		return nil, errors.New("this Pico Password file is damaged")
	}
	v.Items = c.Items
	return v, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// save writes the vault to its file, readable only by the user, going through a temporary
// file so a failure never leaves half a file.
func (v *Vault) save() error {
	plain, err := json.Marshal(contents{Version: fileVersion, Items: v.Items})
	if err != nil {
		return err
	}
	gcm, err := newGCM(v.key)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	env := envelope{Format: fileFormat, Version: fileVersion, KDF: v.kdf, Nonce: nonce, Data: gcm.Seal(nil, nonce, plain, nil)}
	out, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(v.path, append(out, '\n'))
}

// writeFile writes data at path, readable only by the user, through a temporary file.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ---- The master password in the system keychain ----

const (
	keyringService = "picopassword"
	keyringUser    = "master password"
)

// savedPassword is the master password kept in the keychain, or "" if there is none.
func savedPassword() (string, error) {
	p, err := keyring.Get(keyringService, keyringUser)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return p, err
}

func deletePassword() { keyring.Delete(keyringService, keyringUser) }

func savePassword(p string) error {
	if err := keyring.Set(keyringService, keyringUser, p); err != nil {
		return fmt.Errorf("could not keep the password in the keychain: %w", err)
	}
	return nil
}
