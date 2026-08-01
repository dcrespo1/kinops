package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	passwordAlgorithm  = "pbkdf2-sha256"
	passwordIterations = 600_000
	passwordSaltBytes  = 16
	passwordKeyBytes   = 32
)

type passwordVerifier struct {
	iterations int
	salt       []byte
	want       []byte
}

func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password is required")
	}
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, passwordKeyBytes)
	if err != nil {
		return "", fmt.Errorf("derive password hash: %w", err)
	}
	return strings.Join([]string{
		passwordAlgorithm,
		strconv.Itoa(passwordIterations),
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	}, "$"), nil
}

func parsePasswordHash(encoded string) (passwordVerifier, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordAlgorithm {
		return passwordVerifier{}, errors.New("admin password hash must use pbkdf2-sha256")
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 100_000 || iterations > 10_000_000 {
		return passwordVerifier{}, errors.New("admin password hash has invalid iteration count")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) < 16 {
		return passwordVerifier{}, errors.New("admin password hash has invalid salt")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) != passwordKeyBytes {
		return passwordVerifier{}, errors.New("admin password hash has invalid key")
	}
	return passwordVerifier{iterations: iterations, salt: salt, want: want}, nil
}

func (v passwordVerifier) Verify(password string) bool {
	got, err := pbkdf2.Key(sha256.New, password, v.salt, v.iterations, len(v.want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, v.want) == 1
}
