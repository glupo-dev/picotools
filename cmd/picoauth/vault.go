package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
//
// A vault linked to GitHub (version 2) needs also the second key: 32 random bytes kept in the
// keychain of each linked computer and never on GitHub. The key is then HKDF-SHA256 of the
// Argon2id key with the second key, so the copy on GitHub cannot be opened with the master
// password alone. The local copy of a linked computer is the same file as the one on GitHub.

const (
	fileFormat  = "picoauth"
	fileVersion = 2 // the newest version this Pico Auth reads: 1 password only, 2 password + second key
)

// envelope is the file on disk.
type envelope struct {
	Format  string `json:"format"`  // always "picoauth"
	Version int    `json:"version"` // 1, or 2 with the second key
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

var (
	errWrongPassword = errors.New("wrong password")
	errNoSecondKey   = errors.New("this vault is linked to GitHub and needs the second key of this computer, which is not in the keychain")
)

// Vault holds the accounts in memory, with the key to save them again.
type Vault struct {
	Accounts []*Account

	path     string
	password string
	kdf      kdf
	pwKey    []byte // made from the password only
	second   []byte // the second key, nil for a vault not linked to GitHub
	key      []byte // the key that encrypts the file
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
	v := &Vault{path: path, password: password, kdf: k, pwKey: k.key(password)}
	v.setSecond(nil)
	return v, nil
}

// setSecond links the vault to a second key (nil: none): the next save is version 2.
func (v *Vault) setSecond(second []byte) {
	v.second = second
	if second == nil {
		v.key = v.pwKey
		return
	}
	k, err := hkdf.Key(sha256.New, v.pwKey, second, "picoauth vault", 32)
	if err != nil {
		panic(err) // only for a length HKDF cannot give
	}
	v.key = k
}

func (v *Vault) version() int {
	if v.second != nil {
		return 2
	}
	return 1
}

func (k kdf) key(password string) []byte {
	return argon2.IDKey([]byte(password), k.Salt, k.Time, k.Memory, k.Threads, 32)
}

// openVault reads and decrypts the file at path; a wrong password gives errWrongPassword.
// second is the second key from the keychain, if this computer has one.
func openVault(path, password string, second []byte) (*Vault, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decryptVault(raw, path, password, second)
}

// decryptVault opens a vault file already read, from the disk or from GitHub.
func decryptVault(raw []byte, path, password string, second []byte) (*Vault, error) {
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
	v := &Vault{path: path, password: password, kdf: env.KDF, pwKey: env.KDF.key(password)}
	switch {
	case env.Version < 2:
		v.setSecond(nil)
	case second == nil:
		return nil, errNoSecondKey
	default:
		v.setSecond(second)
	}
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
		return nil, errors.New("this Pico Auth file is damaged")
	}
	v.Accounts = c.Accounts
	return v, nil
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
	plain, err := json.Marshal(contents{Version: v.version(), Accounts: v.Accounts})
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
	env := envelope{Format: fileFormat, Version: v.version(), KDF: v.kdf, Nonce: nonce, Data: gcm.Seal(nil, nonce, plain, nil)}
	out, err := json.MarshalIndent(env, "", "  ")
	return append(out, '\n'), err
}

// save writes the vault to its file, readable only by the user, going through a temporary
// file so a failure never leaves half a file.
func (v *Vault) save() error {
	data, err := v.encrypted()
	if err != nil {
		return err
	}
	return writeFile(v.path, data)
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

// ---- The second key in the system keychain ----

const keyringSecond = "second key"

// savedSecond is the second key kept in the keychain, or nil if there is none.
func savedSecond() ([]byte, error) {
	s, err := keyring.Get(keyringService, keyringSecond)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(s)
}

func saveSecond(k []byte) error {
	if err := keyring.Set(keyringService, keyringSecond, base64.StdEncoding.EncodeToString(k)); err != nil {
		return fmt.Errorf("could not keep the second key in the keychain: %w", err)
	}
	return nil
}

// newSecond makes a new random second key.
func newSecond() ([]byte, error) {
	k := make([]byte, 32)
	_, err := rand.Read(k)
	return k, err
}

func deletePassword() { keyring.Delete(keyringService, keyringUser) }
func deleteSecond()   { keyring.Delete(keyringService, keyringSecond) }
