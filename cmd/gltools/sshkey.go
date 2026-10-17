package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/crypto/ssh"
)

var keyNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// generateSSHKey creates an Ed25519 key pair like "ssh-keygen -t ed25519" (without passphrase)
// and saves it in ~/.ssh as id_ed25519_<name> and id_ed25519_<name>.pub.
// Existing files are never overwritten.
func generateSSHKey(name string) ([]Result, error) {
	name = strings.TrimSpace(name)
	if !keyNamePattern.MatchString(name) {
		return nil, errors.New("use only letters, digits, dot, dash and underscore for the name (e.g. github)")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".ssh")
	privPath := filepath.Join(dir, "id_ed25519_"+name)
	pubPath := privPath + ".pub"
	for _, p := range []string{privPath, pubPath} {
		if _, err := os.Stat(p); err == nil {
			return nil, fmt.Errorf("%s already exists, choose another name", p)
		}
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	comment := keyComment()
	block, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return nil, err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, err
	}
	pubLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " " + comment

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := writeNewFile(privPath, pem.EncodeToMemory(block), 0o600); err != nil {
		return nil, err
	}
	if err := writeNewFile(pubPath, []byte(pubLine+"\n"), 0o644); err != nil {
		os.Remove(privPath)
		return nil, err
	}

	return []Result{
		{Name: "Public key", Value: pubLine},
		{Name: "Private key file", Value: privPath},
		{Name: "Public key file", Value: pubPath},
	}, nil
}

// existingSSHKeys lists the public keys already in ~/.ssh with the date of their file, e.g.
// "id_ed25519  2026-05-19  ssh-ed25519  gennaro@mac", so the user can see them before creating a new one.
func existingSSHKeys() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	dir := filepath.Join(home, ".ssh")
	files, _ := filepath.Glob(filepath.Join(dir, "*.pub"))
	if len(files) == 0 {
		return []string{"No keys in " + dir + " yet"}
	}
	type key struct{ name, date, kind, comment string }
	var keys []key
	width := 0
	for _, f := range files {
		k := key{name: strings.TrimSuffix(filepath.Base(f), ".pub"), date: "          "}
		if info, err := os.Stat(f); err == nil {
			k.date = info.ModTime().Format("2006-01-02")
		}
		if data, err := os.ReadFile(f); err == nil {
			fields := strings.Fields(string(data))
			if len(fields) > 0 {
				k.kind = fields[0]
			}
			if len(fields) > 2 {
				k.comment = strings.Join(fields[2:], " ")
			}
		}
		keys = append(keys, k)
		width = max(width, len(k.name))
	}
	lines := []string{fmt.Sprintf("Keys already in %s:", dir)}
	for _, k := range keys {
		lines = append(lines, strings.TrimRight(fmt.Sprintf("  %-*s  %s  %-12s %s", width, k.name, k.date, k.kind, k.comment), " "))
	}
	return lines
}

// keyComment is "user@computer", the same comment ssh-keygen puts at the end of the public key.
func keyComment() string {
	name := "user"
	if u, err := user.Current(); err == nil {
		name = u.Username
		if i := strings.LastIndex(name, `\`); i >= 0 { // Windows: DOMAIN\user
			name = name[i+1:]
		}
	}
	host, _ := os.Hostname()
	host = strings.TrimSuffix(host, ".local")
	if host == "" {
		return name
	}
	return name + "@" + host
}

// writeNewFile writes a file that must not exist yet.
func writeNewFile(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
