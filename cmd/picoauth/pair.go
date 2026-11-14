package main

import (
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// A new computer gets the second key from a linked computer through two small files in the
// repository (docs/picoauth-sync.md, section 6). The two computers never talk to each other.
//
//   - The new computer makes an X25519 key pair and writes requests/<id>.json with its public key.
//     Both screens show a 6-digit code made from that public key: a request written by someone
//     else has a different code.
//   - The linked computer, once approved, writes answers/<id>.json: the second key encrypted with
//     AES-256-GCM, with a key that only the new computer can compute (X25519 between a one-time
//     key of the linked computer and the public key of the request, then HKDF-SHA256).

const (
	requestsDir = "requests"
	answersDir  = "answers"
	// requestLife is how long a request waits: older ones are deleted by the linked computers.
	requestLife = 15 * time.Minute
)

func requestPath(id string) string { return requestsDir + "/" + id + ".json" }
func answerPath(id string) string  { return answersDir + "/" + id + ".json" }

// request is the file a new computer writes.
type request struct {
	PublicKey []byte    `json:"public_key"`
	Created   time.Time `json:"created"`
}

// answer is the file the linked computer writes.
type answer struct {
	PublicKey []byte `json:"public_key"` // the one-time key of the linked computer
	Nonce     []byte `json:"nonce"`
	Data      []byte `json:"data"` // the second key, encrypted
}

// newRequestID is a random name for the request files.
func newRequestID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// pairCode is the 6-digit code of a public key, shown on both computers, like "482 913".
func pairCode(pub []byte) string {
	h := sha256.Sum256(pub)
	n := binary.BigEndian.Uint32(h[:4]) % 1000000
	return fmt.Sprintf("%03d %03d", n/1000, n%1000)
}

func answerKey(shared, linkedPub, newPub []byte, id string) ([]byte, error) {
	salt := append(append([]byte{}, linkedPub...), newPub...)
	return hkdf.Key(sha256.New, shared, salt, "picoauth second key "+id, 32)
}

// sealSecond encrypts the second key for the request with the public key newPub.
func sealSecond(newPub []byte, id string, second []byte) ([]byte, error) {
	peer, err := ecdh.X25519().NewPublicKey(newPub)
	if err != nil {
		return nil, errors.New("the request is damaged")
	}
	eph, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := eph.ECDH(peer)
	if err != nil {
		return nil, err
	}
	key, err := answerKey(shared, eph.PublicKey().Bytes(), newPub, id)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return json.Marshal(answer{PublicKey: eph.PublicKey().Bytes(), Nonce: nonce, Data: gcm.Seal(nil, nonce, second, nil)})
}

// openSecond decrypts the answer with the private key of the request.
func openSecond(priv *ecdh.PrivateKey, id string, raw []byte) ([]byte, error) {
	var a answer
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, errors.New("the answer is damaged")
	}
	peer, err := ecdh.X25519().NewPublicKey(a.PublicKey)
	if err != nil {
		return nil, errors.New("the answer is damaged")
	}
	shared, err := priv.ECDH(peer)
	if err != nil {
		return nil, err
	}
	key, err := answerKey(shared, a.PublicKey, priv.PublicKey().Bytes(), id)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	second, err := gcm.Open(nil, a.Nonce, a.Data, nil)
	if err != nil {
		return nil, errors.New("the answer cannot be opened by this device")
	}
	return second, nil
}
