package database

import (
	"context"
	"database/sql"
	"fmt"

	"gatehouse/model"
	"gatehouse/typed_id"
)

type SystemPrincipal struct {
	ID         string                    `json:"id"`
	Alias      *string                   `json:"alias,omitempty"`
	Name       *string                   `json:"name,omitempty"`
	Enabled    bool                      `json:"enabled"`
	Revision   int                       `json:"revision"`
	Identities []SystemPrincipalIdentity `json:"identities"`
}

type SystemPrincipalIdentity struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	Enabled  bool   `json:"enabled"`
	Revision int    `json:"revision"`
}

func (store *Store) SystemPrincipalsGet(ctx context.Context) (error, []SystemPrincipal) {
	rows, err := store.QueryContext(ctx, `
		SELECT principals.id, principals.alias, principals.name, principals.enabled, principals.revision,
			identities.id, identities.key, identities.enabled, identities.revision
		FROM gatehouse_principals AS principals
		LEFT JOIN gatehouse_identities AS identities ON identities.principal_id = principals.id
		ORDER BY principals.id, identities.id`)
	if err != nil { return fmt.Errorf("list system principals: %w", err), nil }
	defer rows.Close()
	principals := []SystemPrincipal{}
	byID := map[string]int{}
	for rows.Next() {
		var principal SystemPrincipal
		var alias, name, identityID, identityKey sql.NullString
		var identityEnabled sql.NullBool
		var identityRevision sql.NullInt64
		if err := rows.Scan(&principal.ID, &alias, &name, &principal.Enabled, &principal.Revision, &identityID, &identityKey, &identityEnabled, &identityRevision); err != nil {
			return fmt.Errorf("scan system principal: %w", err), nil
		}
		index, exists := byID[principal.ID]
		if !exists {
			if alias.Valid { principal.Alias = &alias.String }
			if name.Valid { principal.Name = &name.String }
			principal.Identities = []SystemPrincipalIdentity{}
			index = len(principals)
			byID[principal.ID] = index
			principals = append(principals, principal)
		}
		if identityID.Valid {
			principals[index].Identities = append(principals[index].Identities, SystemPrincipalIdentity{ID: identityID.String, Key: identityKey.String, Enabled: identityEnabled.Bool, Revision: int(identityRevision.Int64)})
		}
	}
	if err := rows.Err(); err != nil { return fmt.Errorf("iterate system principals: %w", err), nil }
	return nil, principals
}

func (store *Store) SystemPrincipalGet(ctx context.Context, id string) (error, *SystemPrincipal) {
	if !typed_id.Valid(typed_id.Principal, id) { return nil, nil }
	err, principals := store.SystemPrincipalsGet(ctx)
	if err != nil { return err, nil }
	for index := range principals {
		if principals[index].ID == id { return nil, &principals[index] }
	}
	return nil, nil
}

func (store *Store) SystemPrincipalEnabledSet(ctx context.Context, id string, enabled bool) (error, *SystemPrincipal) {
	if !typed_id.Valid(typed_id.Principal, id) { return nil, nil }
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin system principal update: %w", err), nil }
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	var current bool
	var revision int
	err = transaction.QueryRowContext(ctx, `SELECT enabled, revision FROM gatehouse_principals WHERE id = `+placeholder(1), id).Scan(&current, &revision)
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return fmt.Errorf("get system principal for update: %w", err), nil }
	if current != enabled {
		revision++
		if _, err := transaction.ExecContext(ctx, `UPDATE gatehouse_principals SET enabled = `+placeholder(1)+`, revision = `+placeholder(2)+` WHERE id = `+placeholder(3), enabled, revision, id); err != nil {
			return fmt.Errorf("update system principal: %w", err), nil
		}
		if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Ref: model.ActivityEventRef{}, Event: "principal.update", ResourceKind: ActivityResourceKindPrincipal, ResourcePrincipal: &id}, []string{id}); err != nil {
			return fmt.Errorf("append system principal update activity: %w", err), nil
		}
	}
	if err := transaction.Commit(); err != nil { return fmt.Errorf("commit system principal update: %w", err), nil }
	return store.SystemPrincipalGet(ctx, id)
}
