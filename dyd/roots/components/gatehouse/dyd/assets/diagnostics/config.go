package diagnostics

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"
)

func parseConfigFromEnv(raw string) (Config, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Config{}, fmt.Errorf("%s is empty", EnvVar)
	}
	if path, found := strings.CutPrefix(raw, "file:"); found {
		path = strings.TrimSpace(path)
		if path == "" {
			return Config{}, fmt.Errorf("%s file: path is empty", EnvVar)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("read diagnostics file %q: %w", path, err)
		}
		var config Config
		if err := yaml.Unmarshal(contents, &config); err != nil {
			return Config{}, fmt.Errorf("parse diagnostics file %q: %w", path, err)
		}
		return config, nil
	}
	if source, found := strings.CutPrefix(raw, "json:"); found {
		source = strings.TrimSpace(source)
		if source == "" {
			return Config{}, fmt.Errorf("%s json: payload is empty", EnvVar)
		}
		var config Config
		if err := json.Unmarshal([]byte(source), &config); err != nil {
			return Config{}, fmt.Errorf("parse diagnostics JSON: %w", err)
		}
		return config, nil
	}
	return Config{}, fmt.Errorf("%s must use file: or json: prefix", EnvVar)
}
