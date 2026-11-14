package main

import (
	"encoding/base32"
	"testing"
	"time"
)

// The test vectors of RFC 6238, appendix B.
func TestRFC6238(t *testing.T) {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	seeds := map[string]string{
		"SHA1":   enc.EncodeToString([]byte("12345678901234567890")),
		"SHA256": enc.EncodeToString([]byte("12345678901234567890123456789012")),
		"SHA512": enc.EncodeToString([]byte("1234567890123456789012345678901234567890123456789012345678901234")),
	}
	cases := []struct {
		unix int64
		alg  string
		want string
	}{
		{59, "SHA1", "94287082"}, {59, "SHA256", "46119246"}, {59, "SHA512", "90693936"},
		{1111111109, "SHA1", "07081804"}, {1111111109, "SHA256", "68084774"}, {1111111109, "SHA512", "25091201"},
		{1234567890, "SHA1", "89005924"}, {1234567890, "SHA256", "91819424"}, {1234567890, "SHA512", "93441116"},
		{20000000000, "SHA1", "65353130"}, {20000000000, "SHA256", "77737706"}, {20000000000, "SHA512", "47863826"},
	}
	for _, c := range cases {
		a := &Account{Secret: seeds[c.alg], Algorithm: c.alg, Digits: 8, Period: 30}
		got, _ := a.code(time.Unix(c.unix, 0))
		if got != c.want {
			t.Errorf("%s at %d: got %s, want %s", c.alg, c.unix, got, c.want)
		}
	}
}

func TestSecondsLeft(t *testing.T) {
	a := &Account{Secret: "JBSWY3DPEHPK3PXP", Algorithm: "SHA1", Digits: 6, Period: 30}
	if _, left := a.code(time.Unix(61, 0)); left != 29 {
		t.Errorf("left = %d", left)
	}
}

func TestParseInput(t *testing.T) {
	a, err := parseInput("  jbsw y3dp-ehpk 3pxp== ")
	if err != nil || a.Secret != "JBSWY3DPEHPK3PXP" || a.Digits != 6 || a.Period != 30 || a.Algorithm != "SHA1" || a.Kind != kindTOTP {
		t.Fatalf("secret: %+v %v", a, err)
	}

	a, err = parseInput("otpauth://totp/ACME%20Co:john.doe@email.com?secret=HXDMVJECJJWSRB3HWIZR4IFUGFTMXBOZ&issuer=ACME%20Co&algorithm=SHA256&digits=8&period=60")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "ACME Co" || a.User != "john.doe@email.com" || a.Algorithm != "SHA256" || a.Digits != 8 || a.Period != 60 {
		t.Fatalf("uri: %+v", a)
	}
	a, err = parseInput("otpauth://totp/someone?secret=JBSWY3DPEHPK3PXP")
	if err != nil || a.Name != "" || a.User != "someone" {
		t.Fatalf("label only: %+v %v", a, err)
	}

	for _, bad := range []string{"", "not base32 !!", "18", "otpauth://hotp/x?secret=JBSWY3DPEHPK3PXP&counter=1",
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&digits=4", "otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&algorithm=MD5", "otpauth://totp/x"} {
		if _, err := parseInput(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

func TestSpaced(t *testing.T) {
	if spaced("482913") != "482 913" || spaced("12345678") != "1234 5678" {
		t.Error(spaced("482913"), spaced("12345678"))
	}
}
