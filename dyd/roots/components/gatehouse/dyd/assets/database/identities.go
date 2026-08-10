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
	Principal string
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
			INSERT INTO gatehouse_identities (id, principal_id, verifiers, enabled, revision)
			VALUES (` + placeholder(1) + `, ` + placeholder(2) + `, ` + placeholder(3) + `, ` + placeholder(4) + `, ` + placeholder(5) + `)
			ON CONFLICT (id) DO UPDATE SET
				principal_id = excluded.principal_id,
				verifiers = excluded.verifiers,
				enabled = excluded.enabled,
				revision = excluded.revision
			WHERE gatehouse_identities.revision < excluded.revision`
		if store.kind == "postgres" {
			query = strings.Replace(query, placeholder(3), placeholder(3)+"::jsonb", 1)
		}
		if _, err := tx.ExecContext(ctx, query, identity.Id, identity.Principal.Id, string(verifiers), identity.Enabled, identity.Revision); err != nil {
			return fmt.Errorf("provision identity %q: %w", identity.Id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit identity provisioning: %w", err)
	}
	return nil
}

func (store *Store) ActiveIdentityGet(ctx context.Context, id string) (error, *ActiveIdentity) {
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT identities.id, identities.principal_id, identities.verifiers
		FROM gatehouse_identities AS identities
		JOIN gatehouse_principals AS principals ON principals.id = identities.principal_id
		WHERE identities.id = `+placeholder(1)+`
			AND identities.enabled = TRUE
			AND principals.enabled = TRUE
	`, id)
	var active ActiveIdentity
	var verifiers []byte
	if err := row.Scan(&active.ID, &active.Principal, &verifiers); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get active identity %q: %w", id, err), nil
	}
	if err := json.Unmarshal(verifiers, &active.Verifiers); err != nil {
		return fmt.Errorf("decode verifiers for identity %q: %w", id, err), nil
	}
	return nil, &active
}
