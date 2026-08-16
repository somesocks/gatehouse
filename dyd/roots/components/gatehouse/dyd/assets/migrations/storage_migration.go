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

type storageProviderMigrationValue struct {
	ID, Protocol string
	Endpoint, Region, Bucket, AccessKeyID, KeychainID, KeychainVersion, SecretAccessKey any
	Revision int
	Enabled bool
}

func storageProviderMigrationBuilder(providers []config.StorageProvider, keyring *keychain.Keyring) MigrationBuilder {
	resolver := storage.NewSecretKeySourceResolver()
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		revisions, err := migrationStorageProviderRevisions(ctx, session)
		if err != nil {
			return err, ""
		}
		values := make([]storageProviderMigrationValue, 0, len(providers))
		for _, provider := range providers {
			if revisions[provider.ID] >= provider.Revision {
				continue
			}
			value := storageProviderMigrationValue{ID: provider.ID, Revision: provider.Revision, Protocol: provider.Protocol, Enabled: provider.Enabled}
			if provider.Protocol == "embedded" {
				values = append(values, value)
				continue
			}
			secretErr, secret := resolver.Resolve(provider.ID, provider.SecretKeySources)
			if secretErr != nil {
				return secretErr, ""
			}
			reference := model.KeychainRef{Id: *provider.Keychain, Version: 1}
			keyringErr, keys := keyring.Get(ctx, []model.KeychainRef{reference})
			if keyringErr != nil {
				clear(secret)
				return fmt.Errorf("get keychain for storage provider %q: %w", provider.ID, keyringErr), ""
			}
			sealErr, encrypted := keychain.Seal(rand.Reader, keys[reference], []byte("gh=v1|storage-provider="+provider.ID), secret)
			clear(secret)
			clear(keys[reference])
			if sealErr != nil {
				return fmt.Errorf("encrypt secret access key for storage provider %q: %w", provider.ID, sealErr), ""
			}
			value.Endpoint = *provider.Endpoint
			value.Region = *provider.Region
			value.Bucket = *provider.Bucket
			value.AccessKeyID = *provider.AccessKeyID
			value.KeychainID = reference.Id
			value.KeychainVersion = reference.Version
			value.SecretAccessKey = encrypted.String()
			values = append(values, value)
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
			INSERT INTO gatehouse_storage_providers (id, revision, protocol, endpoint, region, bucket, access_key_id, keychain_id, keychain_version, secret_access_key, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Revision }}, {{ sqlLiteral .Protocol }}, {{ sqlLiteral .Endpoint }}, {{ sqlLiteral .Region }}, {{ sqlLiteral .Bucket }}, {{ sqlLiteral .AccessKeyID }}, {{ sqlLiteral .KeychainID }}, {{ sqlLiteral .KeychainVersion }}, {{ sqlLiteral .SecretAccessKey }}, {{ sqlBool .Enabled }})
			ON CONFLICT (id) DO UPDATE SET revision = excluded.revision, protocol = excluded.protocol, endpoint = excluded.endpoint, region = excluded.region, bucket = excluded.bucket, access_key_id = excluded.access_key_id, keychain_id = excluded.keychain_id, keychain_version = excluded.keychain_version, secret_access_key = excluded.secret_access_key, enabled = excluded.enabled
			WHERE gatehouse_storage_providers.revision < excluded.revision;
			{{ end }}
		`, values)
	}
}

func workspaceStorageProviderMigrationBuilder(bindings []config.WorkspaceStorageProvider) MigrationBuilder {
	return templateMigrationBuilder(`
		SELECT 1;
		{{ range . }}
		INSERT INTO gatehouse_workspace_storage_providers (workspace, provider, priority, enabled)
			VALUES ((SELECT id FROM gatehouse_workspaces WHERE key = {{ sqlLiteral .WorkspaceID }}), {{ sqlLiteral .Provider }}, {{ sqlLiteral .Priority }}, {{ sqlBool .Enabled }})
		ON CONFLICT (workspace, provider) DO UPDATE SET priority = excluded.priority, enabled = excluded.enabled;
		{{ end }}
	`, bindings)
}

func migrationStorageProviderRevisions(ctx context.Context, session *MigrationSession) (map[string]int, error) {
	rows, err := session.QueryContext(ctx, `SELECT id, revision FROM gatehouse_storage_providers`)
	if err != nil {
		return nil, fmt.Errorf("get storage provider revisions: %w", err)
	}
	defer rows.Close()
	revisions := map[string]int{}
	for rows.Next() {
		var id string
		var revision int
		if err := rows.Scan(&id, &revision); err != nil {
			return nil, fmt.Errorf("read storage provider revision: %w", err)
		}
		revisions[id] = revision
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read storage provider revisions: %w", err)
	}
	return revisions, nil
}
