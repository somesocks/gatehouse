package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSystemConsumersRejectKeychainSelectors(t *testing.T) {
	for name, body := range map[string]string{
		"http":    "services:\n  http: {keychain: other}\n",
		"agent":   "agent_providers:\n  - alias: api\n    revision: 1\n    protocol: openai-responses\n    endpoint: https://api.example.test/responses\n    api_key: {sources: [env:API_KEY], keychain: other}\n",
		"storage": "storage_providers:\n  - alias: s3\n    revision: 1\n    protocol: s3\n    endpoint: https://s3.example.test\n    region: test\n    bucket: test\n    credentials:\n      access_key_id: test\n      secret_access_key: {sources: [env:SECRET], keychain: other}\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte("api_version: v1\n"+body), 0600); err != nil {
				t.Fatal(err)
			}
			if err, _ := ValidateFile(path); err == nil {
				t.Fatal("system keychain selector was accepted")
			}
		})
	}
}
