package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolveStorageProvidersDefaultsToEmbedded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
workspaces:
  - key: engineering
`), 0o600); err != nil {
		t.Fatal(err)
	}
	err, document := ValidateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err, state := ResolveState(document)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := state.StorageProviders, []StorageProvider{{ID: "embedded", Revision: 1, Protocol: storageProviderProtocolEmbedded, Enabled: true}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("storage providers = %#v, want %#v", got, want)
	}
	if got, want := state.WorkspaceStorageProviders, []WorkspaceStorageProvider{{WorkspaceID: "engineering", Provider: "embedded", Priority: 1, Enabled: true}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("workspace storage providers = %#v, want %#v", got, want)
	}
}

func TestResolveStorageProvidersS3AndWorkspaceBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
storage_providers:
  - id: embedded
    revision: 1
    protocol: embedded
  - id: documents
    revision: 2
    protocol: s3
    endpoint: https://s3.example.test
    region: us-east-1
    bucket: engineering-documents
    credentials:
      access_key_id: example-access-key
      secret_access_key:
        keychain: storage
        sources: [env:DOCUMENTS_SECRET]
workspaces:
  - key: engineering
    storage_providers:
      - provider: documents
        priority: 10
      - provider: embedded
        priority: 1
  - key: archive
    storage_providers: []
`), 0o600); err != nil {
		t.Fatal(err)
	}
	err, document := ValidateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err, state := ResolveState(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.StorageProviders) != 2 || state.StorageProviders[1].ID != "embedded" || state.StorageProviders[0].ID != "documents" || state.StorageProviders[0].Keychain == nil || *state.StorageProviders[0].Keychain != "storage" || len(state.StorageProviders[0].SecretKeySources) != 1 || state.StorageProviders[0].SecretKeySources[0] != "env:DOCUMENTS_SECRET" {
		t.Fatalf("storage providers = %#v", state.StorageProviders)
	}
	want := []WorkspaceStorageProvider{
		{WorkspaceID: "engineering", Provider: "documents", Priority: 10, Enabled: true},
		{WorkspaceID: "engineering", Provider: "embedded", Priority: 1, Enabled: true},
	}
	if !reflect.DeepEqual(state.WorkspaceStorageProviders, want) {
		t.Fatalf("workspace storage providers = %#v, want %#v", state.WorkspaceStorageProviders, want)
	}
}

func TestValidateFileRejectsInvalidStorageProviders(t *testing.T) {
	for name, contents := range map[string]string{
		"embedded S3 configuration": `
api_version: v1
storage_providers:
  - id: embedded
    revision: 1
    protocol: embedded
    bucket: invalid
`,
		"incomplete S3 configuration": `
api_version: v1
storage_providers:
  - id: documents
    revision: 1
    protocol: s3
    endpoint: https://s3.example.test
`,
		"unknown workspace provider": `
api_version: v1
storage_providers: []
workspaces:
  - key: engineering
    storage_providers:
      - provider: missing
        priority: 1
`,
		"nonpositive priority": `
api_version: v1
workspaces:
  - key: engineering
    storage_providers:
      - provider: embedded
        priority: 0
`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if err, _ := ValidateFile(path); err == nil {
				t.Fatal("ValidateFile() accepted invalid storage configuration")
			}
		})
	}
}
