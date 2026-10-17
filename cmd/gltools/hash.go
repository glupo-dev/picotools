package main

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is the work factor: each +1 doubles the time. 12 takes a fraction of a second.
const bcryptCost = 12

// Argon2id settings recommended by RFC 9106: 64 MiB of memory, 3 passes, 4 threads.
const (
	argonMemory  = 64 * 1024 // KiB
	argonTime    = 3
	argonThreads = 4
)

func generateHashes(text string) []Result {
	data := []byte(text)

	md5Sum := md5.Sum(data)
	sha1Sum := sha1.Sum(data)
	sha256Sum := sha256.Sum256(data)
	sha512Sum := sha512.Sum512(data)

	return []Result{
		{Name: "MD5", Value: hex.EncodeToString(md5Sum[:])},
		{Name: "SHA-1", Value: hex.EncodeToString(sha1Sum[:])},
		{Name: "SHA-256", Value: hex.EncodeToString(sha256Sum[:])},
		{Name: "SHA-512", Value: hex.EncodeToString(sha512Sum[:])},
		{Name: "bcrypt", Value: bcryptHash(data)},
		{Name: "Argon2id", Value: argon2idHash(data)},
	}
}

// bcryptHash hashes a password for storing it. The salt is random, so the result
// changes every time; bcrypt only uses the first 72 bytes and refuses longer texts.
func bcryptHash(data []byte) string {
	h, err := bcrypt.GenerateFromPassword(data, bcryptCost)
	if err != nil {
		return "(text longer than 72 bytes, not supported by bcrypt)"
	}
	return string(h)
}

// argon2idHash hashes a password in the standard "$argon2id$v=19$m=…,t=…,p=…$salt$hash"
// form, which includes everything needed to check it later. The salt is random.
func argon2idHash(data []byte) string {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		panic(err)
	}
	key := argon2.IDKey(data, salt, argonTime, argonMemory, argonThreads, 32)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, b64(salt), b64(key))
}
