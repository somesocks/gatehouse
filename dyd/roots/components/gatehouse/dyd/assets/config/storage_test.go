package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveStorageProvidersDefaultsToEmbedded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
workspaces:
  - alias: engineering
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
	if got, want := state.StorageProviders, []StorageProvider{{Alias: "embedded", Revision: 1, Protocol: storageProviderProtocolEmbedded, Enabled: true}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("storage providers = %#v, want %#v", got, want)
	}
	if got, want := state.WorkspaceStorageProviders, []WorkspaceStorageProvider{{WorkspaceID: "engineering", ProviderAlias: "embedded", Revision: 1, Priority: 1, Enabled: true}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("workspace storage providers = %#v, want %#v", got, want)
	}
}

func TestResolveStorageProvidersS3AndWorkspaceBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
storage_providers:
  - alias: embedded
    revision: 1
    protocol: embedded
  - alias: documents
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
  - alias: engineering
    storage_providers:
      - provider: documents
        revision: 1
        priority: 10
      - provider: embedded
        revision: 1
        priority: 1
  - alias: archive
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
	if len(state.StorageProviders) != 2 || state.StorageProviders[1].Alias != "embedded" || state.StorageProviders[0].Alias != "documents" || state.StorageProviders[0].AccessKeyID == nil || *state.StorageProviders[0].AccessKeyID != "example-access-key" || state.StorageProviders[0].Keychain == nil || *state.StorageProviders[0].Keychain != "storage" || len(state.StorageProviders[0].SecretKeySources) != 1 || state.StorageProviders[0].SecretKeySources[0] != "env:DOCUMENTS_SECRET" {
		t.Fatalf("storage providers = %#v", state.StorageProviders)
	}
	want := []WorkspaceStorageProvider{
		{WorkspaceID: "engineering", ProviderAlias: "documents", Revision: 1, Priority: 10, Enabled: true},
		{WorkspaceID: "engineering", ProviderAlias: "embedded", Revision: 1, Priority: 1, Enabled: true},
	}
	if !reflect.DeepEqual(state.WorkspaceStorageProviders, want) {
		t.Fatalf("workspace storage providers = %#v, want %#v", state.WorkspaceStorageProviders, want)
	}
}

func TestResolveStorageProvidersS3AccessKeyIDFromEnvironment(t *testing.T) {
	t.Setenv("S3_ACCESS_KEY_ID", "environment-access-key")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
storage_providers:
  - alias: documents
    revision: 1
    protocol: s3
    endpoint: https://s3.example.test
    region: us-east-1
    bucket: documents
    credentials:
      access_key_id: env:S3_ACCESS_KEY_ID
      secret_access_key:
        sources: [env:DOCUMENTS_SECRET]
`), 0o600); err != nil {
		t.Fatal(err)
	}

	err, document := ValidateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err, providers := ResolveStorageProviders(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0].AccessKeyID == nil || *providers[0].AccessKeyID != "environment-access-key" {
		t.Fatalf("storage providers = %#v", providers)
	}
}

func TestResolveStorageProvidersRejectsEmptyS3AccessKeyIDEnvironment(t *testing.T) {
	t.Setenv("S3_ACCESS_KEY_ID", "")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
storage_providers:
  - alias: documents
    revision: 1
    protocol: s3
    endpoint: https://s3.example.test
    region: us-east-1
    bucket: documents
    credentials:
      access_key_id: env:S3_ACCESS_KEY_ID
      secret_access_key:
        sources: [env:DOCUMENTS_SECRET]
`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err, _ := ValidateFile(path); err == nil || !strings.Contains(err.Error(), "S3_ACCESS_KEY_ID") {
		t.Fatalf("ValidateFile() error = %v, want missing S3_ACCESS_KEY_ID", err)
	}
}

func TestResolveStorageProvidersRejectsUnsetS3AccessKeyIDEnvironment(t *testing.T) {
	const variable = "GATEHOUSE_TEST_UNSET_S3_ACCESS_KEY_ID"
	t.Setenv(variable, "set-before-unset")
	if err := os.Unsetenv(variable); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
storage_providers:
  - alias: documents
    revision: 1
    protocol: s3
    endpoint: https://s3.example.test
    region: us-east-1
    bucket: documents
    credentials:
      access_key_id: env:GATEHOUSE_TEST_UNSET_S3_ACCESS_KEY_ID
      secret_access_key:
        sources: [env:DOCUMENTS_SECRET]
`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err, _ := ValidateFile(path); err == nil || !strings.Contains(err.Error(), variable) {
		t.Fatalf("ValidateFile() error = %v, want missing %s", err, variable)
	}
}

func TestResolveStorageProvidersRejectsMalformedS3AccessKeyIDEnvironmentReference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
api_version: v1
storage_providers:
  - alias: documents
    revision: 1
    protocol: s3
    endpoint: https://s3.example.test
    region: us-east-1
    bucket: documents
    credentials:
      access_key_id: env:BAD-NAME
      secret_access_key:
        sources: [env:DOCUMENTS_SECRET]
`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err, _ := ValidateFile(path); err == nil || !strings.Contains(err.Error(), "env:VARIABLE_NAME") {
		t.Fatalf("ValidateFile() error = %v, want invalid env reference", err)
	}
}

func TestValidateFileRejectsInvalidStorageProviders(t *testing.T) {
	for name, contents := range map[string]string{
		"embedded S3 configuration": `
api_version: v1
storage_providers:
  - alias: embedded
    revision: 1
    protocol: embedded
    bucket: invalid
`,
		"incomplete S3 configuration": `
api_version: v1
storage_providers:
  - alias: documents
    revision: 1
    protocol: s3
    endpoint: https://s3.example.test
`,
		"unknown workspace provider": `
api_version: v1
storage_providers: []
workspaces:
  - alias: engineering
    storage_providers:
      - provider: missing
        priority: 1
`,
		"nonpositive priority": `
api_version: v1
workspaces:
  - alias: engineering
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
