package resource

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gatehouse/config"
	"gatehouse/keychain"
	"gatehouse/model"
)

type Resolver struct {
	keyring   *keychain.Keyring
	readFile  func(string) ([]byte, error)
	lookupEnv func(string) (string, bool)
}

func NewResolver(keyring *keychain.Keyring) *Resolver {
	return newResolver(keyring, os.ReadFile, os.LookupEnv)
}

func newResolver(keyring *keychain.Keyring, readFile func(string) ([]byte, error), lookupEnv func(string) (string, bool)) *Resolver {
	return &Resolver{keyring: keyring, readFile: readFile, lookupEnv: lookupEnv}
}

func (resolver *Resolver) Resolve(ctx context.Context, resources []config.Resource) (error, map[model.ResourceRef][]byte) {
	values := make(map[model.ResourceRef][]byte, len(resources))
	type encryptedResource struct {
		ref       model.ResourceRef
		resource  config.Resource
		encrypted keychain.Encrypted
	}
	encryptedResources := make([]encryptedResource, 0)
	references := make([]model.KeychainRef, 0)
	for _, configured := range resources {
		ref := model.ResourceRef{
			Workspace: model.WorkspaceRef{Id: configured.WorkspaceID},
			Id:        configured.ID,
		}
		switch {
		case strings.HasPrefix(configured.Source, "file:"):
			path := strings.TrimPrefix(configured.Source, "file:")
			if path == "" {
				return fmt.Errorf("resolve resource %q in workspace %q: file source path is empty", configured.ID, configured.WorkspaceID), nil
			}
			value, err := resolver.readFile(path)
			if err != nil {
				return fmt.Errorf("resolve resource %q in workspace %q from file %q: %w", configured.ID, configured.WorkspaceID, path, err), nil
			}
			values[ref] = value
		case strings.HasPrefix(configured.Source, "env:"):
			name := strings.TrimPrefix(configured.Source, "env:")
			if name == "" {
				return fmt.Errorf("resolve resource %q in workspace %q: environment source name is empty", configured.ID, configured.WorkspaceID), nil
			}
			value, ok := resolver.lookupEnv(name)
			if !ok {
				return fmt.Errorf("resolve resource %q in workspace %q: environment variable %q is not set", configured.ID, configured.WorkspaceID, name), nil
			}
			values[ref] = []byte(value)
		case strings.HasPrefix(configured.Source, "gh-enc:"):
			err, encrypted := keychain.ParseResource(configured.Source)
			if err != nil {
				return fmt.Errorf("resolve resource %q in workspace %q: %w", configured.ID, configured.WorkspaceID, err), nil
			}
			encryptedResources = append(encryptedResources, encryptedResource{ref: ref, resource: configured, encrypted: encrypted})
			references = append(references, *encrypted.Key)
		default:
			return fmt.Errorf("resolve resource %q in workspace %q: unsupported source %q", configured.ID, configured.WorkspaceID, configured.Source), nil
		}
	}

	err, keys := resolver.keyring.Get(ctx, references)
	if err != nil {
		return err, nil
	}
	for _, configured := range encryptedResources {
		key, ok := keys[*configured.encrypted.Key]
		if !ok {
			return fmt.Errorf("resolve resource %q in workspace %q: keychain key %q version %d is unavailable", configured.resource.ID, configured.resource.WorkspaceID, configured.encrypted.Key.Id, configured.encrypted.Key.Version), nil
		}
		err, value := keychain.Open(key, resourceAssociatedData(configured.resource), configured.encrypted)
		clear(key)
		if err != nil {
			return fmt.Errorf("resolve resource %q in workspace %q: %w", configured.resource.ID, configured.resource.WorkspaceID, err), nil
		}
		values[configured.ref] = value
	}
	return nil, values
}

func resourceAssociatedData(resource config.Resource) []byte {
	return []byte("gh=v1|workspace=" + resource.WorkspaceID + "|resource=" + resource.ID)
}
