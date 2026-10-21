// Package kdf defines the work factors encoded by Gatehouse KDF algorithm IDs.
package kdf

import "fmt"

const (
	// InsecureV0 is a one-iteration profile for disposable test data.
	InsecureV0 = "pbkdf2-hmac-sha256-v0"
	Default    = "pbkdf2-hmac-sha256-v1"
)

func Iterations(algorithm string) (int, error) {
	switch algorithm {
	case InsecureV0:
		return 1, nil
	case Default:
		return 600_000, nil
	default:
		return 0, fmt.Errorf("unsupported KDF algorithm %q", algorithm)
	}
}
