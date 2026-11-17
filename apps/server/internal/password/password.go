// Package password implements the HydroOJ-compatible password hash and the
// legacy bcrypt verification needed while old hashes are migrated.
package password

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"math/big"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
)

const (
	iterations = 100000
	keyLength  = 64

	// SaltLength is the length of the random salt stored with a password hash.
	SaltLength = 32

	// saltAlphabet is the exact character set HydroOJ uses for salts.
	saltAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"
)

var saltAlphabetSize = big.NewInt(int64(len(saltAlphabet)))

// HydroHash derives the password hash exactly as HydroOJ does: PBKDF2-HMAC-
// SHA256 with 100000 iterations and a 64-byte key, hex-encoded and truncated to
// its first 64 characters.
func HydroHash(password, salt string) string {
	key := pbkdf2.Key([]byte(password), []byte(salt), iterations, keyLength, sha256.New)
	return hex.EncodeToString(key)[:64]
}

// GenerateSalt returns a 32-character random salt drawn uniformly from
// HydroOJ's 62-character alphabet.
func GenerateSalt() (string, error) {
	salt := make([]byte, SaltLength)
	for i := range salt {
		n, err := rand.Int(rand.Reader, saltAlphabetSize)
		if err != nil {
			return "", err
		}
		salt[i] = saltAlphabet[n.Int64()]
	}
	return string(salt), nil
}

// Hash generates a new salt and returns the HydroOJ password hash together
// with that salt.
func Hash(password string) (hash, salt string, err error) {
	salt, err = GenerateSalt()
	if err != nil {
		return "", "", err
	}
	return HydroHash(password, salt), salt, nil
}

// IsLegacyBcrypt reports whether hash is a bcrypt hash from before the
// HydroOJ-compatible scheme. HydroOJ hashes are 64 hex characters and can
// never start with "$".
func IsLegacyBcrypt(hash string) bool {
	return strings.HasPrefix(hash, "$2")
}

// Verify checks password against a stored hash and salt. Legacy bcrypt hashes
// are verified with bcrypt; every other hash uses the HydroOJ algorithm and a
// constant-time comparison.
func Verify(password, salt, hash string) bool {
	if IsLegacyBcrypt(hash) {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	}
	expected := HydroHash(password, salt)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(hash)) == 1
}
