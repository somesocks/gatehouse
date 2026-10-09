package config

import (
	"fmt"
	"os"
	"strings"
)

func resolveEnvironmentReference(value, field string) (string, error) {
	if !strings.HasPrefix(value, "env:") {
		return value, nil
	}
	if !environmentReference.MatchString(value) {
		return "", fmt.Errorf("%s must be an env:VARIABLE_NAME reference", field)
	}

	name := strings.TrimPrefix(value, "env:")
	resolved, exists := os.LookupEnv(name)
	if !exists || strings.TrimSpace(resolved) == "" {
		return "", fmt.Errorf("%s references unset or empty environment variable %q", field, name)
	}
	return resolved, nil
}
