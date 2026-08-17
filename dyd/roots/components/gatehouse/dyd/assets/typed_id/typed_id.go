// Package typed_id creates and validates durable, typed identifiers.
package typed_id

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
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
	StorageObject   = "obj"
	SessionFile     = "sfi"
	Gateway         = "gwy"
	Project         = "prj"
	Session         = "ses"
	SessionEvent    = "sev"
	ActivityEvent = "act"
)

func New(kind string) (string, error) {
	return NewAt(kind, time.Now().UTC())
}

// NewAt creates a typed ULID with the given timestamp.
func NewAt(kind string, at time.Time) (string, error) {
	if !validKind(kind) {
		return "", fmt.Errorf("invalid typed ID kind %q", kind)
	}
	id, err := ulid.New(ulid.Timestamp(at), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate typed ID: %w", err)
	}
	return kind + "_" + strings.ToLower(id.String()), nil
}

func Valid(kind, value string) bool {
	_, err := Timestamp(kind, value)
	return err == nil
}

// Timestamp returns the timestamp encoded in a typed ULID.
func Timestamp(kind, value string) (time.Time, error) {
	if !validKind(kind) || !strings.HasPrefix(value, kind+"_") {
		return time.Time{}, fmt.Errorf("invalid typed ID kind %q", kind)
	}
	encoded := strings.TrimPrefix(value, kind+"_")
	if encoded != strings.ToLower(encoded) {
		return time.Time{}, fmt.Errorf("typed ID must be lower-case")
	}
	id, err := ulid.ParseStrict(strings.ToUpper(encoded))
	if err != nil || strings.ToLower(id.String()) != encoded {
		return time.Time{}, fmt.Errorf("invalid typed ID")
	}
	return id.Timestamp().UTC(), nil
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
