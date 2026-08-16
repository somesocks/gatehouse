package config

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"gatehouse/configschema"
)

const (
	storageProviderProtocolEmbedded = "embedded"
	storageProviderProtocolS3       = "s3"
	implicitStorageProviderID       = "embedded"
)

type StorageProvider struct {
	ID                string
	Revision          int
	Protocol          string
	Endpoint          *string
	Region            *string
	Bucket            *string
	AccessKeyID       *string
	Keychain          *string
	SecretKeySources  []StorageProviderSecretKeySource
	Enabled           bool
}

type StorageProviderSecretKeySource string

type WorkspaceStorageProvider struct {
	WorkspaceID string
	Provider    string
	Priority    int
	Enabled     bool
}

func ResolveStorageProviders(document configschema.GatehouseConfig) (error, []StorageProvider) {
	if document.StorageProviders == nil {
		return nil, []StorageProvider{{ID: implicitStorageProviderID, Revision: 1, Protocol: storageProviderProtocolEmbedded, Enabled: true}}
	}

	providers := make([]StorageProvider, 0, len(*document.StorageProviders))
	ids := make(map[string]struct{}, len(*document.StorageProviders))
	for index, configured := range *document.StorageProviders {
		if !keychainID.MatchString(configured.Id) || configured.Revision <= 0 {
			return fmt.Errorf("storage_providers[%d] has an invalid ID or revision", index), nil
		}
		if _, exists := ids[configured.Id]; exists {
			return fmt.Errorf("storage_providers[%d].id %q is duplicated", index, configured.Id), nil
		}
		enabled := configured.Enabled == nil || *configured.Enabled
		provider := StorageProvider{ID: configured.Id, Revision: configured.Revision, Protocol: configured.Protocol, Enabled: enabled}
		switch configured.Protocol {
		case storageProviderProtocolEmbedded:
			if configured.Endpoint != nil || configured.Region != nil || configured.Bucket != nil || configured.Credentials != nil {
				return fmt.Errorf("storage_providers[%d] embedded providers do not accept S3 configuration", index), nil
			}
		case storageProviderProtocolS3:
			if configured.Endpoint == nil || configured.Region == nil || configured.Bucket == nil || configured.Credentials == nil {
				return fmt.Errorf("storage_providers[%d] S3 providers require endpoint, region, bucket, and credentials", index), nil
			}
			parsed, err := url.ParseRequestURI(*configured.Endpoint)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				return fmt.Errorf("storage_providers[%d].endpoint must be an absolute HTTP URL", index), nil
			}
			if strings.TrimSpace(*configured.Region) == "" || strings.TrimSpace(*configured.Bucket) == "" || strings.TrimSpace(configured.Credentials.AccessKeyId) == "" {
				return fmt.Errorf("storage_providers[%d] S3 region, bucket, and access key ID must not be blank", index), nil
			}
			keychain := defaultKeychainID
			if configured.Credentials.SecretAccessKey.Keychain != nil {
				keychain = *configured.Credentials.SecretAccessKey.Keychain
			}
			if !keychainID.MatchString(keychain) || len(configured.Credentials.SecretAccessKey.Sources) == 0 {
				return fmt.Errorf("storage_providers[%d].credentials.secret_access_key is invalid", index), nil
			}
			sources := make([]StorageProviderSecretKeySource, 0, len(configured.Credentials.SecretAccessKey.Sources))
			seen := make(map[string]struct{}, len(configured.Credentials.SecretAccessKey.Sources))
			for sourceIndex, source := range configured.Credentials.SecretAccessKey.Sources {
				if _, exists := seen[source]; exists {
					return fmt.Errorf("storage_providers[%d].credentials.secret_access_key.sources[%d] %q is duplicated", index, sourceIndex, source), nil
				}
				if !environmentReference.MatchString(source) && source != "stdin:" {
					return fmt.Errorf("storage_providers[%d].credentials.secret_access_key.sources[%d] is invalid", index, sourceIndex), nil
				}
				seen[source] = struct{}{}
				sources = append(sources, StorageProviderSecretKeySource(source))
			}
			provider.Endpoint = configured.Endpoint
			provider.Region = configured.Region
			provider.Bucket = configured.Bucket
			provider.AccessKeyID = &configured.Credentials.AccessKeyId
			provider.Keychain = &keychain
			provider.SecretKeySources = sources
		default:
			return fmt.Errorf("storage_providers[%d] has an invalid protocol", index), nil
		}
		ids[provider.ID] = struct{}{}
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(left, right int) bool { return providers[left].ID < providers[right].ID })
	return nil, providers
}

func ResolveWorkspaceStorageProviders(document configschema.GatehouseConfig, providers []StorageProvider) (error, []WorkspaceStorageProvider) {
	configuredProviders := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		configuredProviders[provider.ID] = struct{}{}
	}
	if document.Workspaces == nil {
		if _, exists := configuredProviders[implicitStorageProviderID]; exists {
			return nil, []WorkspaceStorageProvider{{WorkspaceID: defaultWorkspaceKey, Provider: implicitStorageProviderID, Priority: 1, Enabled: true}}
		}
		return nil, []WorkspaceStorageProvider{}
	}

	bindings := []WorkspaceStorageProvider{}
	for workspaceIndex, workspace := range *document.Workspaces {
		if !workspaceKey.MatchString(workspace.Key) || workspace.Key == gatehouseWorkspaceKey {
			return fmt.Errorf("workspaces[%d].key is not a configurable workspace", workspaceIndex), nil
		}
		if workspace.StorageProviders == nil {
			if _, exists := configuredProviders[implicitStorageProviderID]; exists {
				bindings = append(bindings, WorkspaceStorageProvider{WorkspaceID: workspace.Key, Provider: implicitStorageProviderID, Priority: 1, Enabled: true})
			}
			continue
		}
		seen := make(map[string]struct{}, len(*workspace.StorageProviders))
		for providerIndex, configured := range *workspace.StorageProviders {
			if !keychainID.MatchString(configured.Provider) {
				return fmt.Errorf("workspaces[%d].storage_providers[%d].provider must match %q", workspaceIndex, providerIndex, keychainID.String()), nil
			}
			if _, exists := configuredProviders[configured.Provider]; !exists {
				return fmt.Errorf("workspaces[%d].storage_providers[%d].provider %q is not configured", workspaceIndex, providerIndex, configured.Provider), nil
			}
			if _, exists := seen[configured.Provider]; exists {
				return fmt.Errorf("workspaces[%d].storage_providers[%d].provider %q is duplicated", workspaceIndex, providerIndex, configured.Provider), nil
			}
			if configured.Priority <= 0 {
				return fmt.Errorf("workspaces[%d].storage_providers[%d].priority must be positive", workspaceIndex, providerIndex), nil
			}
			enabled := configured.Enabled == nil || *configured.Enabled
			seen[configured.Provider] = struct{}{}
			bindings = append(bindings, WorkspaceStorageProvider{WorkspaceID: workspace.Key, Provider: configured.Provider, Priority: configured.Priority, Enabled: enabled})
		}
	}
	sort.Slice(bindings, func(left, right int) bool {
		if bindings[left].WorkspaceID == bindings[right].WorkspaceID {
			return bindings[left].Provider < bindings[right].Provider
		}
		return bindings[left].WorkspaceID < bindings[right].WorkspaceID
	})
	return nil, bindings
}
