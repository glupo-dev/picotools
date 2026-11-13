package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/argon2"
)

// The vault is one file with every account, encrypted with a key made from the master password.
// On disk it is a small JSON envelope: how the key is made (Argon2id with its salt and costs),
// and the accounts encrypted with AES-256-GCM. Nothing about the accounts is readable without
// the password, not even their names.

const (
	fileFormat  = "picoauth"
	fileVersion = 1
)

// envelope is the file on disk.
type envelope struct {
	Format  string `json:"format"`  // always "picoauth"
	Version int    `json:"version"` // fileVersion
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
	Version  int        `json:"version"`
	Accounts []*Account `json:"accounts"`
}

// Argon2id costs for new vaults: about a tenth of a second on a laptop.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 4
)

var errWrongPassword = errors.New("wrong password")

// Vault holds the accounts in memory, with the key to save them again.
type Vault struct {
	Accounts []*Account

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
	return filepath.Join(dir, "picoauth", "vault.picoauth"), nil
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
// The vault keeps path as its file, so a backup opened from elsewhere can be saved in place with moveTo.
func openVault(path, password string) (*Vault, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil || env.Format != fileFormat {
		return nil, errors.New("this is not a Pico Auth file")
	}
	if env.Version > fileVersion {
		return nil, errors.New("this file was written by a newer Pico Auth: update the app")
	}
	if env.KDF.Name != "argon2id" || len(env.KDF.Salt) == 0 || env.KDF.Memory == 0 {
		return nil, errors.New("this Pico Auth file is damaged")
	}
	key := env.KDF.key(password)
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, env.Nonce, env.Data, nil)
	if err != nil {
		return nil, errWrongPassword // or the file was changed: GCM cannot tell the two apart
	}
	var c contents
	if err := json.Unmarshal(plain, &c); err != nil {
		return nil, errors.New("this Pico Auth file is damaged")
	}
	return &Vault{Accounts: c.Accounts, path: path, password: password, kdf: env.KDF, key: key}, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// encrypted is the file as it is written: the accounts encrypted with a new nonce.
func (v *Vault) encrypted() ([]byte, error) {
	plain, err := json.Marshal(contents{Version: fileVersion, Accounts: v.Accounts})
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(v.key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	env := envelope{Format: fileFormat, Version: fileVersion, KDF: v.kdf, Nonce: nonce, Data: gcm.Seal(nil, nonce, plain, nil)}
	return json.MarshalIndent(env, "", "  ")
}

// save writes the vault to its file, readable only by the user.
func (v *Vault) save() error {
	return v.writeTo(v.path)
}

// writeTo writes the encrypted vault to path, going through a temporary file so a
// failure never leaves half a file.
func (v *Vault) writeTo(path string) error {
	data, err := v.encrypted()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// moveTo makes path the vault's file and saves it there.
func (v *Vault) moveTo(path string) error {
	v.path = path
	return v.save()
}

// checkPassword says whether password is the master password, taking the same time either way.
func (v *Vault) checkPassword(password string) bool {
	return subtle.ConstantTimeCompare([]byte(password), []byte(v.password)) == 1
}

// ---- The master password in the system keychain ----

const (
	keyringService = "picoauth"
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

func savePassword(p string) error {
	if err := keyring.Set(keyringService, keyringUser, p); err != nil {
		return fmt.Errorf("could not keep the password in the keychain: %w", err)
	}
	return nil
}
