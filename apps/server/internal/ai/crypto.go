package ai

import "github.com/kaptree/kapoj/apps/server/internal/secret"

// Encryptor, NewEncryptor and ErrInvalidKey are kept as aliases so the AI
// domain and its existing callers keep working while the AES-GCM primitive is
// shared with mail settings.
type Encryptor = secret.Encryptor

var NewEncryptor = secret.NewEncryptor

var ErrInvalidKey = secret.ErrInvalidKey
