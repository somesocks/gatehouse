package resource

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
)

func TestResolverResolvesBatchSources(t *testing.T) {
	keyring := openResourceKeyring(t)
	defer keyring.Close()
	reference := model.KeychainRef{Id: "default", Version: 1}
	err, keys := keyring.Get(context.Background(), []model.KeychainRef{reference})
	if err != nil {
		t.Fatal(err)
	}
	encrypted := encryptResource(t, keys[reference], config.Resource{WorkspaceID: "engineering", ID: "token"}, []byte("encrypted"), reference)
	resolver := newResolver(keyring, func(path string) ([]byte, error) {
		if path != "./endpoint" {
			t.Fatalf("read file path = %q, want ./endpoint", path)
		}
		return []byte{}, nil
	}, func(name string) (string, bool) {
		if name != "EMPTY" {
			t.Fatalf("environment name = %q, want EMPTY", name)
		}
		return "", true
	})
	resources := []config.Resource{
		{WorkspaceID: "engineering", ID: "endpoint", Source: "file:./endpoint"},
		{WorkspaceID: "engineering", ID: "empty", Source: "env:EMPTY"},
		{WorkspaceID: "engineering", ID: "token", Source: encrypted.String()},
	}
	err, values := resolver.Resolve(context.Background(), resources)
	if err != nil {
		t.Fatal(err)
	}
	if got := values[model.ResourceRef{Workspace: model.WorkspaceRef{Id: "engineering"}, Id: "endpoint"}]; len(got) != 0 {
		t.Fatalf("file resource = %q, want empty value", got)
	}
	if got := values[model.ResourceRef{Workspace: model.WorkspaceRef{Id: "engineering"}, Id: "empty"}]; len(got) != 0 {
		t.Fatalf("environment resource = %q, want empty value", got)
	}
	if got, want := values[model.ResourceRef{Workspace: model.WorkspaceRef{Id: "engineering"}, Id: "token"}], []byte("encrypted"); !bytes.Equal(got, want) {
		t.Fatalf("encrypted resource = %q, want %q", got, want)
	}
}

func TestResolverRejectsIdentityMismatchTamperingAndMissingKey(t *testing.T) {
	keyring := openResourceKeyring(t)
	defer keyring.Close()
	reference := model.KeychainRef{Id: "default", Version: 1}
	err, keys := keyring.Get(context.Background(), []model.KeychainRef{reference})
	if err != nil {
		t.Fatal(err)
	}
	resource := config.Resource{WorkspaceID: "engineering", ID: "token"}
	encrypted := encryptResource(t, keys[reference], resource, []byte("encrypted"), reference)
	tampered := encrypted
	tampered.Payload = append([]byte(nil), encrypted.Payload...)
	tampered.Payload[len(tampered.Payload)-1] ^= 1
	missing := keychain.Encrypted{
		Payload: bytes.Repeat([]byte{1}, 28),
		Key:     &model.KeychainRef{Id: "default", Version: 2},
	}
	resolver := newResolver(keyring, nil, nil)
	tests := []struct {
		name     string
		resource config.Resource
		contains string
	}{
		{
			name:     "identity mismatch",
			resource: config.Resource{WorkspaceID: "engineering", ID: "other", Source: encrypted.String()},
			contains: "decrypt encrypted payload",
		},
		{
			name:     "tampered",
			resource: config.Resource{WorkspaceID: "engineering", ID: "token", Source: tampered.String()},
			contains: "decrypt encrypted payload",
		},
		{
			name:     "missing key version",
			resource: config.Resource{WorkspaceID: "engineering", ID: "token", Source: missing.String()},
			contains: "not found",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err, _ := resolver.Resolve(context.Background(), []config.Resource{test.resource})
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Resolve() error = %v, want %q", err, test.contains)
			}
		})
	}
}

func TestResourceAssociatedData(t *testing.T) {
	if got, want := string(resourceAssociatedData(config.Resource{WorkspaceID: "engineering", ID: "token"})), "gh=v1|workspace=engineering|resource=token"; got != want {
		t.Fatalf("resourceAssociatedData() = %q, want %q", got, want)
	}
}

func openResourceKeyring(t *testing.T) *keychain.Keyring {
	t.Helper()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	t.Setenv("RESOURCE_KEYCHAIN", "passphrase")
	configured := []config.Keychain{{
		ID:      "default",
		Sources: []config.KeychainPassphraseSource{"env:RESOURCE_KEYCHAIN"},
	}}
	err, keyring := keychain.NewKeyring(store, configured, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(keyring.Close)
	err, set := migrations.Build(configuration, config.State{Keychains: configured}, keyring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(context.Background(), store, set); err != nil {
		t.Fatal(err)
	}
	return keyring
}

func encryptResource(t *testing.T, key []byte, resource config.Resource, plaintext []byte, reference model.KeychainRef) keychain.Encrypted {
	t.Helper()
	err, encrypted := keychain.Seal(bytes.NewReader(bytes.Repeat([]byte{1}, 12)), key, resourceAssociatedData(resource), plaintext)
	if err != nil {
		t.Fatal(err)
	}
	encrypted.Key = &reference
	return encrypted
}
