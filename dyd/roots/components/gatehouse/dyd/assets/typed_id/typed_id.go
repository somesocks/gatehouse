// Package typed_id creates and validates durable, typed identifiers.
package typed_id

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"strings"
)

const (
	Workspace     = "wsp"
	Principal     = "prn"
	Identity      = "idt"
	Group         = "grp"
	Tool          = "tol"
	Resource      = "res"
	AgentProvider = "apr"
	AgentModel    = "amd"
	StorageProvider = "stp"
	Session         = "ses"
	SessionEvent    = "sev"
	ActivityEvent   = "act"
	encodedLength   = 26
)

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

func New(kind string) (string, error) {
	if !validKind(kind) {
		return "", fmt.Errorf("invalid typed ID kind %q", kind)
	}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate typed ID: %w", err)
	}
	return kind + "_" + strings.ToLower(encoding.EncodeToString(bytes)), nil
}

// Derive returns a deterministic typed ID from stable input bytes.
func Derive(kind, input string) (string, error) {
	if !validKind(kind) {
		return "", fmt.Errorf("invalid typed ID kind %q", kind)
	}
	sum := sha256.Sum256([]byte(input))
	return kind + "_" + strings.ToLower(encoding.EncodeToString(sum[:16])), nil
}

func Valid(kind, value string) bool {
	if !validKind(kind) || !strings.HasPrefix(value, kind+"_") {
		return false
	}
	encoded := strings.TrimPrefix(value, kind+"_")
	if len(encoded) != encodedLength || encoded != strings.ToLower(encoded) {
		return false
	}
	decoded, err := encoding.DecodeString(strings.ToUpper(encoded))
	return err == nil && len(decoded) == 16 && strings.ToLower(encoding.EncodeToString(decoded)) == encoded
}

func validKind(kind string) bool {
	if len(kind) == 0 {
		return false
	}
	for _, character := range kind {
		if character < 'a' || character > 'z' {
			return false
		}
	}
	return true
}
