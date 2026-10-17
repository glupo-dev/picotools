package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const (
	lower   = "abcdefghijklmnopqrstuvwxyz"
	upper   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits  = "0123456789"
	symbols = "!@#$%&*?-_+="

	// upperClear has no I, L, O: easy to read and dictate without mistakes.
	upperClear = "ABCDEFGHJKMNPQRSTUVWXYZ"
)

type passwordLevel struct {
	name     string
	length   int
	generate func() string
}

var passwordLevels = []passwordLevel{
	{"Temporary", 8, func() string { return randomPassword(8, []string{upperClear}) }},
	{"Strong", 20, strongPassword},
	{"Very strong", 32, func() string { return randomPassword(32, []string{lower, upper, digits, symbols}) }},
	// Letters and digits only, so it is safe in URLs, headers and config files.
	{"API key", 40, func() string { return randomPassword(40, []string{lower, upper, digits}) }},
	// 256 bit secret, like "openssl rand -hex 32" (JWT, cookies, webhooks, encryption keys).
	{"Hex secret", 64, func() string { return hex.EncodeToString(randomBytes(32)) }},
	{"UUID v4", 36, uuidV4},
	{"UUID v7", 36, uuidV7},
}

func generatePasswords() []Result {
	results := make([]Result, 0, len(passwordLevels))
	for _, lvl := range passwordLevels {
		results = append(results, Result{
			Name:  fmt.Sprintf("%-11s (%2d)", lvl.name, lvl.length),
			Value: lvl.generate(),
		})
	}
	return results
}

// strongPassword builds a 20 character password like "kxr4ab.ATvz7r_gwa0jp":
// three groups of 6 lowercase letters and digits joined by "." "_" or "-",
// with a few letters turned uppercase. Mostly lowercase, so it is easy to type
// on any keyboard, and still around 95 bits of randomness.
func strongPassword() string {
	const groupChars = lower + digits
	const separators = "._-"
	for {
		b := make([]byte, 0, 20)
		for g := 0; g < 3; g++ {
			if g > 0 {
				b = append(b, separators[randomInt(len(separators))])
			}
			for i := 0; i < 6; i++ {
				b = append(b, groupChars[randomInt(len(groupChars))])
			}
		}
		for n := 1 + randomInt(3); n > 0; n-- {
			i := randomInt(len(b))
			if b[i] >= 'a' && b[i] <= 'z' {
				b[i] -= 'a' - 'A'
			}
		}
		if pwd := string(b); containsAll(pwd, []string{lower, upper, digits}) {
			return pwd
		}
	}
}

// uuidV4 returns a random UUID (version 4), e.g. "3f2b8c1e-9a4d-4f6b-8e2a-1c5d7b9e0f3a".
func uuidV4() string {
	b := randomBytes(16)
	return formatUUID(b, 4)
}

// uuidV7 returns a UUID (version 7) that starts with the current time in milliseconds,
// so UUIDs created later sort after earlier ones (good as database keys).
func uuidV7() string {
	b := randomBytes(16)
	ms := time.Now().UnixMilli()
	for i := 0; i < 6; i++ {
		b[i] = byte(ms >> (40 - 8*i))
	}
	return formatUUID(b, 7)
}

// formatUUID sets the version and the RFC 9562 variant and writes b in the usual form.
func formatUUID(b []byte, version byte) string {
	b[6] = b[6]&0x0f | version<<4
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

// randomPassword builds a password with a secure random generator
// and makes sure it contains at least one character from every required group.
func randomPassword(length int, sets []string) string {
	all := strings.Join(sets, "")
	for {
		b := make([]byte, length)
		for i := range b {
			b[i] = all[randomInt(len(all))]
		}
		pwd := string(b)
		if containsAll(pwd, sets) {
			return pwd
		}
	}
}

func containsAll(s string, sets []string) bool {
	for _, set := range sets {
		if !strings.ContainsAny(s, set) {
			return false
		}
	}
	return true
}

func randomInt(max int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		panic(err)
	}
	return int(n.Int64())
}
