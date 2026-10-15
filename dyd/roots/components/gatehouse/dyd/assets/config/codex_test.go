package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexProviderConfiguration(t *testing.T) {
	for _, test := range []struct {
		name, extra string
		valid       bool
	}{
		{"default keychain", "", true},
		{"selected keychain", "    credential: {keychain: oauth}\n", true},
		{"fixed endpoint", "    endpoint: " + OpenAICodexEndpoint + "\n", true},
		{"custom endpoint", "    endpoint: https://other.test/responses\n", false},
		{"source import", "    credential: {sources: [env:TOKEN]}\n", false},
		{"legacy api key", "    api_key: {sources: [env:TOKEN]}\n", false},
		{"ambiguous credential", "    api_key: {sources: [env:TOKEN]}\n    credential: {keychain: oauth}\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte("api_version: v1\nagent_providers:\n  - alias: chatgpt\n    revision: 1\n    protocol: openai-codex\n"+test.extra), 0600); err != nil {
				t.Fatal(err)
			}
			err, document := ValidateFile(path)
			if err == nil {
				err, _ = ResolveAgentProviders(document)
			}
			if (err == nil) != test.valid {
				t.Fatalf("validation = %v", err)
			}
			if test.valid {
				_, providers := ResolveAgentProviders(document)
				if len(providers) != 1 || *providers[0].Endpoint != OpenAICodexEndpoint || providers[0].Keychain == nil || len(providers[0].Sources) != 0 {
					t.Fatalf("resolved = %#v", providers)
				}
				if strings.Contains(test.extra, "oauth") && *providers[0].Keychain != "oauth" {
					t.Fatal("keychain selection lost")
				}
			}
		})
	}
}

func TestGenericAgentCredentialKeepsAPIKeySources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("api_version: v1\nagent_providers:\n  - alias: api\n    revision: 1\n    protocol: openai-responses\n    endpoint: https://api.example.test/responses\n    credential: {keychain: api, sources: [env:API_KEY]}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err, document := ValidateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err, providers := ResolveAgentProviders(document)
	if err != nil || len(providers) != 1 || *providers[0].Keychain != "api" || len(providers[0].Sources) != 1 || providers[0].Sources[0] != "env:API_KEY" {
		t.Fatalf("credentials = %#v %v", providers, err)
	}
}
