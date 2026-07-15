// Package authutil handles password hashing with only the Go
// standard library — no golang.org/x/crypto — so the project has
// zero external module dependencies and builds anywhere.
package authutil

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

const (
	pbkdf2Iterations = 120_000
	saltBytes        = 16
	keyLen           = 32
)

// HashPassword returns a self-describing hash string of the form
// "pbkdf2$<iterations>$<saltHex>$<hashHex>" so the iteration count
// and salt travel with the hash — no separate config needed to verify later.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := pbkdf2(password, salt, pbkdf2Iterations, keyLen)
	return fmt.Sprintf("pbkdf2$%d$%s$%s", pbkdf2Iterations, hex.EncodeToString(salt), hex.EncodeToString(hash)), nil
}

// VerifyPassword checks a plaintext password against a hash produced by HashPassword.
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	salt, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got := pbkdf2(password, salt, iterations, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// pbkdf2 is a minimal PBKDF2 implementation (RFC 8018) using
// HMAC-SHA256, since golang.org/x/crypto/pbkdf2 isn't in stdlib.
func pbkdf2(password string, salt []byte, iterations, keyLen int) []byte {
	prf := hmac.New(sha256.New, []byte(password))
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen

	var derived []byte
	buf := make([]byte, 4)
	for block := 1; block <= numBlocks; block++ {
		buf[0] = byte(block >> 24)
		buf[1] = byte(block >> 16)
		buf[2] = byte(block >> 8)
		buf[3] = byte(block)

		prf.Reset()
		prf.Write(salt)
		prf.Write(buf)
		u := prf.Sum(nil)
		t := make([]byte, len(u))
		copy(t, u)

		for i := 1; i < iterations; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		derived = append(derived, t...)
	}
	return derived[:keyLen]
}
