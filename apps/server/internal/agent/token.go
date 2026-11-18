package agent

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// TokenPrefix starts every Agent API token so leaked tokens are recognizable.
const TokenPrefix = "kapoj_agent_"

// GenerateToken returns a fresh Agent token. The plaintext is returned exactly
// once to the caller; only the hex SHA-256 hash and a short prefix are stored.
func GenerateToken() (plain, hash, prefix string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", err
	}
	plain = TokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	return plain, HashToken(plain), plain[:20], nil
}

// HashToken hashes a raw token with SHA-256 and returns lowercase hex.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
