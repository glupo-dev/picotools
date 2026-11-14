package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Kinds of entry. Only TOTP for now; the field lets the file hold other kinds later.
const kindTOTP = "totp"

// Account is one login with its TOTP secret, as the service gave it.
type Account struct {
	Kind      string `json:"kind"`           // kindTOTP
	Name      string `json:"name"`           // the service, e.g. GitHub
	User      string `json:"user,omitempty"` // the account on the service, e.g. an email
	Secret    string `json:"secret"`         // Base32, upper case, no spaces or padding
	Algorithm string `json:"algorithm"`      // SHA1, SHA256 or SHA512
	Digits    int    `json:"digits"`         // 6, 7 or 8
	Period    int    `json:"period"`         // seconds, usually 30
}

// The values used when the service does not say otherwise.
const (
	defaultAlgorithm = "SHA1"
	defaultDigits    = 6
	defaultPeriod    = 30
)

// code is the code of the account at time t (RFC 6238), and how many seconds it is still valid.
func (a *Account) code(t time.Time) (string, int) {
	key, err := decodeSecret(a.Secret)
	if err != nil {
		return "", 0
	}
	period := int64(a.Period)
	if period <= 0 {
		period = defaultPeriod
	}
	digits := a.Digits
	if digits <= 0 {
		digits = defaultDigits
	}
	unix := t.Unix()
	return hotp(key, uint64(unix/period), digits, a.Algorithm), int(period - unix%period)
}

// hotp is the code for one counter value (RFC 4226).
func hotp(key []byte, counter uint64, digits int, algorithm string) string {
	var newHash func() hash.Hash
	switch algorithm {
	case "SHA256":
		newHash = sha256.New
	case "SHA512":
		newHash = sha512.New
	default:
		newHash = sha1.New
	}
	mac := hmac.New(newHash, key)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	mod := uint32(1)
	for range digits {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, n%mod)
}

// spaced splits a code in two halves to read it more easily: "482913" → "482 913".
func spaced(code string) string {
	if len(code) < 6 {
		return code
	}
	return code[:len(code)/2] + " " + code[len(code)/2:]
}

// cleanSecret turns a secret as services show it ("jbsw y3dp ehpk 3pxp") into plain Base32.
func cleanSecret(s string) (string, error) {
	s = strings.ToUpper(strings.NewReplacer(" ", "", "-", "", "\t", "", "=", "").Replace(s))
	if s == "" {
		return "", errors.New("the secret is empty")
	}
	if _, err := decodeSecret(s); err != nil {
		return "", errors.New("this is not a valid secret: it should be letters A-Z and digits 2-7")
	}
	return s, nil
}

func decodeSecret(s string) ([]byte, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err == nil && len(key) == 0 {
		err = errors.New("empty secret")
	}
	return key, err
}

// parseInput reads what was pasted: either a secret alone or an otpauth://totp/ URI.
func parseInput(s string) (*Account, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToLower(s), "otpauth://") {
		return parseURI(s)
	}
	secret, err := cleanSecret(s)
	if err != nil {
		return nil, err
	}
	return &Account{Kind: kindTOTP, Secret: secret, Algorithm: defaultAlgorithm, Digits: defaultDigits, Period: defaultPeriod}, nil
}

// parseURI reads otpauth://totp/Issuer:user?secret=...&issuer=...&algorithm=...&digits=...&period=...
func parseURI(s string) (*Account, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, errors.New("this is not a valid otpauth:// address")
	}
	if !strings.EqualFold(u.Host, "totp") {
		return nil, errors.New("only time-based codes (otpauth://totp/) are supported")
	}
	q := u.Query()
	secret, err := cleanSecret(q.Get("secret"))
	if err != nil {
		return nil, err
	}
	a := &Account{Kind: kindTOTP, Secret: secret, Algorithm: defaultAlgorithm, Digits: defaultDigits, Period: defaultPeriod}

	label := strings.TrimPrefix(u.Path, "/")
	issuer, user, found := strings.Cut(label, ":")
	if !found {
		issuer, user = "", label
	}
	a.Name = strings.TrimSpace(issuer)
	a.User = strings.TrimSpace(user)
	if i := strings.TrimSpace(q.Get("issuer")); i != "" {
		a.Name = i
	}

	if v := strings.ToUpper(q.Get("algorithm")); v != "" {
		if v != "SHA1" && v != "SHA256" && v != "SHA512" {
			return nil, errors.New("unknown algorithm " + v)
		}
		a.Algorithm = v
	}
	if v := q.Get("digits"); v != "" {
		d, err := strconv.Atoi(v)
		if err != nil || d < 6 || d > 8 {
			return nil, errors.New("digits must be 6, 7 or 8")
		}
		a.Digits = d
	}
	if v := q.Get("period"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 300 {
			return nil, errors.New("the period must be between 1 and 300 seconds")
		}
		a.Period = p
	}
	return a, nil
}
