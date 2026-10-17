package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Claims that hold a date as seconds since 1970.
var jwtTimeClaims = map[string]bool{"exp": true, "iat": true, "nbf": true, "auth_time": true}

// decodeJWT shows what is inside a JWT. It only reads it: the signature is not checked,
// because that needs the key of whoever created the token.
func decodeJWT(token string) ([]Result, error) {
	token = strings.TrimSpace(token)
	token = strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("this is not a JWT: it should have three parts separated by dots")
	}
	header, err := jwtPart(parts[0])
	if err != nil {
		return nil, fmt.Errorf("cannot read the header: %w", err)
	}
	payload, err := jwtPart(parts[1])
	if err != nil {
		return nil, fmt.Errorf("cannot read the payload: %w", err)
	}

	var head map[string]any
	var claims map[string]any
	json.Unmarshal(header, &head)
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, errors.New("the payload is not a JSON object")
	}

	results := []Result{
		{Name: "Status", Value: jwtStatus(claims) + " · signature not verified"},
		{Name: "Header", Value: compactJSON(header)},
		{Name: "Payload", Value: compactJSON(payload)},
	}
	if alg, ok := head["alg"].(string); ok {
		results = append(results, Result{Name: "alg", Value: alg})
	}

	keys := make([]string, 0, len(claims))
	for k := range claims {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		results = append(results, Result{Name: k, Value: claimValue(k, claims[k])})
	}
	return results, nil
}

// jwtPart decodes one part of the token (base64url, with or without padding).
func jwtPart(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

// jwtStatus says whether the token is expired or not yet valid, based on exp and nbf.
func jwtStatus(claims map[string]any) string {
	now := time.Now()
	if nbf, ok := claimTime(claims["nbf"]); ok && now.Before(nbf) {
		return "not valid yet, starts " + relative(nbf)
	}
	exp, ok := claimTime(claims["exp"])
	switch {
	case !ok:
		return "no expiry"
	case now.After(exp):
		return "expired " + relative(exp)
	default:
		return "valid, expires " + relative(exp)
	}
}

func claimValue(key string, v any) string {
	if t, ok := claimTime(v); ok && jwtTimeClaims[key] {
		return t.Local().Format("2006-01-02 15:04:05") + " (" + relative(t) + ")"
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func claimTime(v any) (time.Time, bool) {
	f, ok := v.(float64)
	if !ok {
		return time.Time{}, false
	}
	sec, frac := math.Modf(f)
	return time.Unix(int64(sec), int64(frac*1e9)), true
}

// relative describes a moment as "in 2 hours" or "3 days ago".
func relative(t time.Time) string {
	d := time.Until(t)
	future := d > 0
	if !future {
		d = -d
	}
	var s string
	switch {
	case d < time.Minute:
		s = "less than a minute"
	case d < time.Hour:
		s = plural(int(d.Minutes()), "minute")
	case d < 48*time.Hour:
		s = plural(int(d.Hours()), "hour")
	default:
		s = plural(int(d.Hours()/24), "day")
	}
	if future {
		return "in " + s
	}
	return s + " ago"
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

func compactJSON(b []byte) string {
	var buf bytes.Buffer
	if json.Compact(&buf, b) != nil {
		return string(b)
	}
	return buf.String()
}
