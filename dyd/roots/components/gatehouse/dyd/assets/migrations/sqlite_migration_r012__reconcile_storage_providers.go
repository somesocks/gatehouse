package migrations

import (
	"context"
	"crypto/rand"
	"fmt"

	"gatehouse/config"
	"gatehouse/keychain"
	"gatehouse/model"
	"gatehouse/storage"
)

func sqliteMigrationR012ReconcileStorageProviders(state config.State, keyring *keychain.Keyring) RepeatableMigration {
	return RepeatableMigration{
		Index:       12,
		Description: "reconcile_storage_providers",
		Builder:     sqliteMigrationR012ReconcileStorageProvidersBuilder(state.StorageProviders, keyring),
	}
}

type sqliteMigrationR012StorageProviderValue struct {
	Alias, Protocol                                                               string
	Endpoint, Region, Bucket, AccessKeyID, KeychainID, KeychainVersion, SecretKey any
	Revision                                                                      int
	Enabled                                                                       bool
}

func sqliteMigrationR012ReconcileStorageProvidersBuilder(providers []config.StorageProvider, keyring *keychain.Keyring) MigrationBuilder {
	resolver := storage.NewSecretKeySourceResolver()
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		revisions, err := sqliteMigrationR012StorageProviderRevisionsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		values := make([]sqliteMigrationR012StorageProviderValue, 0, len(providers))
		for _, provider := range providers {
			if revisions[provider.Alias] >= provider.Revision {
				continue
			}
			value := sqliteMigrationR012StorageProviderValue{Alias: provider.Alias, Revision: provider.Revision, Protocol: provider.Protocol, Enabled: provider.Enabled}
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
			DROP TABLE IF EXISTS gatehouse_migration_storage_provider_desired;
			DROP TABLE IF EXISTS gatehouse_migration_storage_provider_state;
			DROP TABLE IF EXISTS gatehouse_migration_storage_provider_activities;

			CREATE TEMP TABLE gatehouse_migration_storage_provider_desired (
				alias TEXT NOT NULL PRIMARY KEY,
				revision INTEGER NOT NULL,
				protocol TEXT NOT NULL,
				endpoint TEXT,
				region TEXT,
				bucket TEXT,
				access_key_id TEXT,
				keychain_id TEXT,
				keychain_version INTEGER,
				secret_access_key TEXT,
				enabled INTEGER NOT NULL
			) STRICT;
			{{ range . }}
			INSERT INTO gatehouse_migration_storage_provider_desired (alias, revision, protocol, endpoint, region, bucket, access_key_id, keychain_id, keychain_version, secret_access_key, enabled)
			VALUES ({{ sqlLiteral .Alias }}, {{ sqlLiteral .Revision }}, {{ sqlLiteral .Protocol }}, {{ sqlLiteral .Endpoint }}, {{ sqlLiteral .Region }}, {{ sqlLiteral .Bucket }}, {{ sqlLiteral .AccessKeyID }}, {{ sqlLiteral .KeychainID }}, {{ sqlLiteral .KeychainVersion }}, {{ sqlLiteral .SecretKey }}, {{ sqlBool .Enabled }});
			{{ end }}

			CREATE TEMP TABLE gatehouse_migration_storage_provider_state AS
			SELECT desired.*, COALESCE(providers.id, gh_id_new('stp')) AS id, providers.id AS existing_id
			FROM gatehouse_migration_storage_provider_desired AS desired
			LEFT JOIN gatehouse_storage_providers AS providers ON providers.alias = desired.alias;

			CREATE TEMP TABLE gatehouse_migration_storage_provider_activities (
				provider TEXT NOT NULL,
				id TEXT NOT NULL,
				event TEXT NOT NULL
			) STRICT;
			INSERT INTO gatehouse_migration_storage_provider_activities (provider, id, event)
			SELECT id, gh_id_new('act'), CASE WHEN existing_id IS NULL THEN 'storage_provider.create' ELSE 'storage_provider.update' END
			FROM gatehouse_migration_storage_provider_state;

			INSERT INTO gatehouse_storage_providers (id, alias, revision, protocol, endpoint, region, bucket, access_key_id, keychain_id, keychain_version, secret_access_key, enabled)
			SELECT id, alias, revision, protocol, endpoint, region, bucket, access_key_id, keychain_id, keychain_version, secret_access_key, enabled
			FROM gatehouse_migration_storage_provider_state
			WHERE TRUE
			ON CONFLICT (alias) DO UPDATE SET revision = excluded.revision, protocol = excluded.protocol, endpoint = excluded.endpoint, region = excluded.region, bucket = excluded.bucket, access_key_id = excluded.access_key_id, keychain_id = excluded.keychain_id, keychain_version = excluded.keychain_version, secret_access_key = excluded.secret_access_key, enabled = excluded.enabled
			WHERE gatehouse_storage_providers.revision < excluded.revision;

			INSERT INTO gatehouse_activity_events (
				id, event, resource_kind, resource_storage_provider, created_at
			)
			SELECT id, event, 'storage_provider', provider, gh_id_timestamp(id)
			FROM gatehouse_migration_storage_provider_activities;
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			SELECT id, 'sys/' || provider FROM gatehouse_migration_storage_provider_activities;

			DROP TABLE gatehouse_migration_storage_provider_activities;
			DROP TABLE gatehouse_migration_storage_provider_state;
			DROP TABLE gatehouse_migration_storage_provider_desired;
		`, values)
	}
}

func sqliteMigrationR012StorageProviderRevisionsByAlias(ctx context.Context, session *MigrationSession) (map[string]int, error) {
	rows, err := session.QueryContext(ctx, `SELECT alias, revision FROM gatehouse_storage_providers WHERE alias IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("get storage provider revisions: %w", err)
	}
	defer rows.Close()

	revisions := map[string]int{}
	for rows.Next() {
		var alias string
		var revision int
		if err := rows.Scan(&alias, &revision); err != nil {
			return nil, fmt.Errorf("read storage provider: %w", err)
		}
		revisions[alias] = revision
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read storage providers: %w", err)
	}
	return revisions, nil
}
