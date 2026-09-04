package migrations

import (
	"context"

	"gatehouse/keychain"
)

func sqliteMigrationR001PrepareKeychains(keyring *keychain.Keyring) RepeatableMigration {
	return RepeatableMigration{
		Index:       1,
		Description: "prepare_keychains",
		Builder:     sqliteMigrationR001PrepareKeychainsBuilder(keyring),
	}
}

func sqliteMigrationR001PrepareKeychainsBuilder(keyring *keychain.Keyring) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		err, candidates := keyring.Candidates()
		if err != nil {
			return err, ""
		}
		return session.RenderTemplate(`
			DROP TABLE IF EXISTS gatehouse_migration_keychain_activities;
			CREATE TEMP TABLE gatehouse_migration_keychain_activities (
				keychain_id TEXT NOT NULL,
				keychain_version INTEGER NOT NULL,
				id TEXT NOT NULL
			) STRICT;
			{{ range . }}
			INSERT INTO gatehouse_migration_keychain_activities (keychain_id, keychain_version, id)
			SELECT {{ sqlLiteral .Ref.Id }}, {{ sqlLiteral .Ref.Version }}, gh_id_new('act')
			WHERE NOT EXISTS (
				SELECT 1 FROM gatehouse_keychains
				WHERE id = {{ sqlLiteral .Ref.Id }} AND version = {{ sqlLiteral .Ref.Version }}
			);
			{{ end }}
			{{ range . }}
			INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled)
			VALUES ({{ sqlLiteral .Ref.Id }}, {{ sqlLiteral .Ref.Version }}, {{ sqlLiteral .KekKdf }}, {{ sqlLiteral .Key }}, {{ sqlBool .Enabled }})
			ON CONFLICT (id, version) DO NOTHING;
			{{ end }}

			INSERT INTO gatehouse_activity_events (
				id, event, resource_kind, resource_keychain_id, resource_keychain_version, created_at
			)
			SELECT id, 'keychain.create', 'keychain', keychain_id, keychain_version, gh_id_timestamp(id)
			FROM gatehouse_migration_keychain_activities;
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			SELECT id, 'sys/kch/' || keychain_id || '/' || keychain_version
			FROM gatehouse_migration_keychain_activities;

			DROP TABLE gatehouse_migration_keychain_activities;
		`, candidates)
	}
}
