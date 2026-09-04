package migrations

import (
	"context"
	"strconv"

	"gatehouse/keychain"
	"gatehouse/model"
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
		existing, err := postgresMigrationR001Keychains(ctx, session)
		if err != nil {
			return err, ""
		}
		events := make([]activityMigrationEvent, 0, len(candidates))
		for _, candidate := range candidates {
			if existing[candidate.Ref] {
				continue
			}
			event, err := newActivityMigrationEvent("", "keychain.create", "keychain", "", "", "", "sys/kch/"+candidate.Ref.Id+"/"+strconv.Itoa(candidate.Ref.Version))
			if err != nil {
				return err, ""
			}
			event.KeychainID = candidate.Ref.Id
			event.KeychainVersion = candidate.Ref.Version
			events = append(events, event)
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range .Candidates }}
			INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled)
			VALUES ({{ sqlLiteral .Ref.Id }}, {{ sqlLiteral .Ref.Version }}, {{ sqlLiteral .KekKdf }}, {{ sqlLiteral .Key }}, {{ sqlBool .Enabled }})
			ON CONFLICT (id, version) DO NOTHING;
			{{ end }}
			{{ range .Events }}
			{{ $event := . }}
			INSERT INTO gatehouse_activity_events (
				id, event, resource_kind, resource_keychain_id, resource_keychain_version, created_at
			) VALUES (
				{{ sqlLiteral .ID }}, {{ sqlLiteral .Event }}, {{ sqlLiteral .ResourceKind }}, {{ sqlLiteral .KeychainID }}, {{ sqlLiteral .KeychainVersion }}, {{ sqlLiteral .CreatedAt }}
			);
			{{ range .Topics }}
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			VALUES ({{ sqlLiteral $event.ID }}, {{ sqlLiteral . }});
			{{ end }}
			{{ end }}
		`, struct {
			Candidates []model.Keychain
			Events     []activityMigrationEvent
		}{Candidates: candidates, Events: events})
	}
}

func postgresMigrationR001Keychains(ctx context.Context, session *MigrationSession) (map[model.KeychainRef]bool, error) {
	rows, err := session.QueryContext(ctx, `SELECT id, version FROM gatehouse_keychains`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keychains := map[model.KeychainRef]bool{}
	for rows.Next() {
		var reference model.KeychainRef
		if err := rows.Scan(&reference.Id, &reference.Version); err != nil {
			return nil, err
		}
		keychains[reference] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return keychains, nil
}
