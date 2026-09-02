package migrations

import (
	"context"
	"crypto/rand"
	"fmt"

	"gatehouse/config"
	"gatehouse/keychain"
	"gatehouse/model"
	"gatehouse/storage"
	"gatehouse/typed_id"
)

func postgresMigrationR012ReconcileStorageProviders(state config.State, keyring *keychain.Keyring) RepeatableMigration {
	return RepeatableMigration{
		Index:       12,
		Description: "reconcile_storage_providers",
		Builder:     postgresMigrationR012ReconcileStorageProvidersBuilder(state.StorageProviders, keyring),
	}
}

type postgresMigrationR012StorageProviderValue struct {
	ID, Alias, Protocol                                                           string
	Endpoint, Region, Bucket, AccessKeyID, KeychainID, KeychainVersion, SecretKey any
	Revision                                                                      int
	Enabled                                                                       bool
}

type postgresMigrationR012StorageProviderRevision struct {
	ID       string
	Revision int
}

func postgresMigrationR012ReconcileStorageProvidersBuilder(providers []config.StorageProvider, keyring *keychain.Keyring) MigrationBuilder {
	resolver := storage.NewSecretKeySourceResolver()
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		existing, err := postgresMigrationR012StorageProvidersByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		values := make([]postgresMigrationR012StorageProviderValue, 0, len(providers))
		for _, provider := range providers {
			previous := existing[provider.Alias]
			if previous.Revision >= provider.Revision {
				continue
			}
			id := previous.ID
			if id == "" {
				id, err = typed_id.New(typed_id.StorageProvider)
				if err != nil {
					return err, ""
				}
			}
			value := postgresMigrationR012StorageProviderValue{ID: id, Alias: provider.Alias, Revision: provider.Revision, Protocol: provider.Protocol, Enabled: provider.Enabled}
			if provider.Protocol == "embedded" {
				values = append(values, value)
				continue
			}
			secretErr, secret := resolver.Resolve(provider.Alias, provider.SecretKeySources)
			if secretErr != nil {
				return secretErr, ""
			}
			reference := model.KeychainRef{Id: *provider.Keychain, Version: 1}
			keyringErr, keys := keyring.Get(ctx, []model.KeychainRef{reference})
			if keyringErr != nil {
				clear(secret)
				return fmt.Errorf("get keychain for storage provider %q: %w", provider.Alias, keyringErr), ""
			}
			sealErr, encrypted := keychain.Seal(rand.Reader, keys[reference], []byte("gh=v1|storage-provider|alias="+provider.Alias), secret)
			clear(secret)
			clear(keys[reference])
			if sealErr != nil {
				return fmt.Errorf("encrypt secret access key for storage provider %q: %w", provider.Alias, sealErr), ""
			}
			encrypted.AAD = keychain.AADAlias
			value.Endpoint = *provider.Endpoint
			value.Region = *provider.Region
			value.Bucket = *provider.Bucket
			value.AccessKeyID = *provider.AccessKeyID
			value.KeychainID = reference.Id
			value.KeychainVersion = reference.Version
			value.SecretKey = encrypted.String()
			values = append(values, value)
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
			INSERT INTO gatehouse_storage_providers (id, alias, revision, protocol, endpoint, region, bucket, access_key_id, keychain_id, keychain_version, secret_access_key, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Alias }}, {{ sqlLiteral .Revision }}, {{ sqlLiteral .Protocol }}, {{ sqlLiteral .Endpoint }}, {{ sqlLiteral .Region }}, {{ sqlLiteral .Bucket }}, {{ sqlLiteral .AccessKeyID }}, {{ sqlLiteral .KeychainID }}, {{ sqlLiteral .KeychainVersion }}, {{ sqlLiteral .SecretKey }}, {{ sqlBool .Enabled }})
			ON CONFLICT (alias) DO UPDATE SET revision = excluded.revision, protocol = excluded.protocol, endpoint = excluded.endpoint, region = excluded.region, bucket = excluded.bucket, access_key_id = excluded.access_key_id, keychain_id = excluded.keychain_id, keychain_version = excluded.keychain_version, secret_access_key = excluded.secret_access_key, enabled = excluded.enabled
			WHERE gatehouse_storage_providers.revision < excluded.revision;
			{{ end }}
		`, values)
	}
}

func postgresMigrationR012StorageProvidersByAlias(ctx context.Context, session *MigrationSession) (map[string]postgresMigrationR012StorageProviderRevision, error) {
	rows, err := session.QueryContext(ctx, `SELECT alias, id, revision FROM gatehouse_storage_providers WHERE alias IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("get storage provider revisions: %w", err)
	}
	defer rows.Close()

	providers := map[string]postgresMigrationR012StorageProviderRevision{}
	for rows.Next() {
		var alias string
		var provider postgresMigrationR012StorageProviderRevision
		if err := rows.Scan(&alias, &provider.ID, &provider.Revision); err != nil {
			return nil, fmt.Errorf("read storage provider: %w", err)
		}
		providers[alias] = provider
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read storage providers: %w", err)
	}
	return providers, nil
}
