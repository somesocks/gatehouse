package migrations

import (
	"context"

	"gatehouse/keychain"
)

func postgresMigrationR001PrepareKeychains(keyring *keychain.Keyring) RepeatableMigration {
	return RepeatableMigration{
		Index:       1,
		Description: "prepare_keychains",
		Builder:     postgresMigrationR001PrepareKeychainsBuilder(keyring),
	}
}

func postgresMigrationR001PrepareKeychainsBuilder(keyring *keychain.Keyring) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		err, candidates := keyring.Candidates()
		if err != nil {
			return err, ""
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
			INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled)
			VALUES ({{ sqlLiteral .Ref.Id }}, {{ sqlLiteral .Ref.Version }}, {{ sqlLiteral .KekKdf }}, {{ sqlLiteral .Key }}, {{ sqlBool .Enabled }})
			ON CONFLICT (id, version) DO NOTHING;
			{{ end }}
		`, candidates)
	}
}
