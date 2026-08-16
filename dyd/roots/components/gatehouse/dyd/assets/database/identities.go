package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"gatehouse/model"
)

type ActiveIdentity struct {
	ID        string
	Key       string
	Principal model.Principal
	Verifiers []interface{}
}

func (store *Store) IdentitiesUpsertRevisions(ctx context.Context, identities []model.Identity) error {
	if len(identities) == 0 {
		return nil
	}

	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin identity provisioning: %w", err)
	}
	defer tx.Rollback()

	placeholder := keychainPlaceholder(store.kind)
	for _, identity := range identities {
		verifiers, err := json.Marshal(identity.Verifiers)
		if err != nil {
			return fmt.Errorf("encode verifiers for identity %q: %w", identity.Id, err)
		}
		query := `
			INSERT INTO gatehouse_identities (id, alias, key, principal_id, verifiers, enabled, revision)
			VALUES (` + placeholder(1) + `, ` + placeholder(2) + `, ` + placeholder(3) + `, ` + placeholder(4) + `, ` + placeholder(5) + `, ` + placeholder(6) + `, ` + placeholder(7) + `)
			ON CONFLICT (id) DO UPDATE SET
				alias = excluded.alias,
				key = excluded.key,
				principal_id = excluded.principal_id,
				verifiers = excluded.verifiers,
				enabled = excluded.enabled,
				revision = excluded.revision
			WHERE gatehouse_identities.revision < excluded.revision`
		if store.kind == "postgres" {
			query = strings.Replace(query, placeholder(5), placeholder(5)+"::jsonb", 1)
		}
		if _, err := tx.ExecContext(ctx, query, identity.Id, identity.Alias, identity.Key, identity.Principal.Id, string(verifiers), identity.Enabled, identity.Revision); err != nil {
			return fmt.Errorf("provision identity %q: %w", identity.Id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit identity provisioning: %w", err)
	}
	return nil
}

func (store *Store) ActiveIdentityGetByID(ctx context.Context, id string) (error, *ActiveIdentity) {
	return store.activeIdentityGet(ctx, "identities.id", id)
}

func (store *Store) ActiveIdentityGetByKey(ctx context.Context, key string) (error, *ActiveIdentity) {
	return store.activeIdentityGet(ctx, "identities.key", key)
}

func (store *Store) activeIdentityGet(ctx context.Context, field, value string) (error, *ActiveIdentity) {
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT identities.id, identities.key, principals.id, principals.alias, principals.name, principals.enabled, identities.verifiers
		FROM gatehouse_identities AS identities
		JOIN gatehouse_principals AS principals ON principals.id = identities.principal_id
		WHERE `+field+` = `+placeholder(1)+`
			AND identities.enabled = TRUE
			AND principals.enabled = TRUE
	`, value)
	var active ActiveIdentity
	var alias, name sql.NullString
	var verifiers []byte
	if err := row.Scan(&active.ID, &active.Key, &active.Principal.Ref.Id, &alias, &name, &active.Principal.Enabled, &verifiers); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get active identity %q: %w", value, err), nil
	}
	if alias.Valid {
		active.Principal.Alias = &alias.String
	}
	if name.Valid {
		active.Principal.Name = &name.String
	}
	if err := json.Unmarshal(verifiers, &active.Verifiers); err != nil {
		return fmt.Errorf("decode verifiers for identity %q: %w", value, err), nil
	}
	return nil, &active
}
