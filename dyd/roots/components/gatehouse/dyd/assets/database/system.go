package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gatehouse/authz"
	"gatehouse/model"
	"gatehouse/typed_id"
)

var (
	ErrSystemGrantPrincipalNotFound = errors.New("system grant principal not found")
	ErrSystemGrantAlreadyExists     = errors.New("system grant already exists")
)

func (store *Store) SystemRolesGet(ctx context.Context, principal model.PrincipalRef) (error, []authz.Role) {
	if !typed_id.Valid(typed_id.Principal, principal.Id) {
		return nil, []authz.Role{}
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT grants.role
		FROM gatehouse_system_grants AS grants
		JOIN gatehouse_principals AS principals ON principals.id = grants.principal
		WHERE grants.principal = `+placeholder(1)+` AND grants.enabled = TRUE AND principals.enabled = TRUE
	`, principal.Id)
	if err != nil {
		return fmt.Errorf("get system roles: %w", err), nil
	}
	defer rows.Close()
	roles := []authz.Role{}
	for rows.Next() {
		var role authz.Role
		if err := rows.Scan(&role); err != nil { return fmt.Errorf("scan system role: %w", err), nil }
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil { return fmt.Errorf("iterate system roles: %w", err), nil }
	return nil, roles
}

func (store *Store) SystemGrantsGet(ctx context.Context) (error, []model.SystemGrant) {
	rows, err := store.QueryContext(ctx, `SELECT id, principal, enabled, revision FROM gatehouse_system_grants ORDER BY id`)
	if err != nil { return fmt.Errorf("list system grants: %w", err), nil }
	defer rows.Close()
	grants := []model.SystemGrant{}
	for rows.Next() {
		var grant model.SystemGrant
		if err := rows.Scan(&grant.Ref.Id, &grant.Principal.Id, &grant.Enabled, &grant.Revision); err != nil { return fmt.Errorf("scan system grant: %w", err), nil }
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil { return fmt.Errorf("iterate system grants: %w", err), nil }
	return nil, grants
}

func (store *Store) SystemGrantGet(ctx context.Context, id string) (error, *model.SystemGrant) {
	if !typed_id.Valid(typed_id.SystemGrant, id) { return nil, nil }
	var grant model.SystemGrant
	err := store.QueryRowContext(ctx, `SELECT id, principal, enabled, revision FROM gatehouse_system_grants WHERE id = `+keychainPlaceholder(store.kind)(1), id).Scan(&grant.Ref.Id, &grant.Principal.Id, &grant.Enabled, &grant.Revision)
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return fmt.Errorf("get system grant: %w", err), nil }
	return nil, &grant
}

func (store *Store) SystemGrantCreate(ctx context.Context, principal model.PrincipalRef) (error, *model.SystemGrant) {
	if !typed_id.Valid(typed_id.Principal, principal.Id) { return fmt.Errorf("create system grant: principal ID is invalid"), nil }
	id, err := typed_id.New(typed_id.SystemGrant)
	if err != nil { return fmt.Errorf("generate system grant ID: %w", err), nil }
	grant := model.SystemGrant{Ref: model.SystemGrantRef{Id: id}, Principal: principal, Enabled: true, Revision: 1}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin system grant creation: %w", err), nil }
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	var storedPrincipal string
	if err := transaction.QueryRowContext(ctx, `SELECT id FROM gatehouse_principals WHERE id = `+placeholder(1), principal.Id).Scan(&storedPrincipal); err == sql.ErrNoRows {
		return ErrSystemGrantPrincipalNotFound, nil
	} else if err != nil {
		return fmt.Errorf("get system grant principal: %w", err), nil
	}
	var insertedID string
	err = transaction.QueryRowContext(ctx, `
		INSERT INTO gatehouse_system_grants (id, principal, role, enabled, revision)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, TRUE, 1)
		ON CONFLICT (principal) DO NOTHING
		RETURNING id
	`, grant.Ref.Id, principal.Id, authz.Manager).Scan(&insertedID)
	if err == sql.ErrNoRows {
		return ErrSystemGrantAlreadyExists, nil
	}
	if err != nil {
		return fmt.Errorf("insert system grant: %w", err), nil
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Ref: model.ActivityEventRef{}, Event: "system_grant.create", ResourceKind: ActivityResourceKindSystemGrant, ResourceSystemGrant: &grant.Ref.Id}, []string{ActivityTopicSystemGrant(grant.Ref), ActivityTopicPrincipalGrant(principal, grant.Ref.Id)}); err != nil {
		return fmt.Errorf("append system grant creation activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil { return fmt.Errorf("commit system grant creation: %w", err), nil }
	return nil, &grant
}

func (store *Store) SystemGrantEnabledSet(ctx context.Context, id string, enabled bool) (error, *model.SystemGrant) {
	if !typed_id.Valid(typed_id.SystemGrant, id) { return nil, nil }
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin system grant update: %w", err), nil }
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	grant := model.SystemGrant{Ref: model.SystemGrantRef{Id: id}}
	err = transaction.QueryRowContext(ctx, `SELECT principal, enabled, revision FROM gatehouse_system_grants WHERE id = `+placeholder(1), id).Scan(&grant.Principal.Id, &grant.Enabled, &grant.Revision)
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return fmt.Errorf("get system grant for update: %w", err), nil }
	if grant.Enabled == enabled {
		if err := transaction.Commit(); err != nil { return fmt.Errorf("commit unchanged system grant: %w", err), nil }
		return nil, &grant
	}
	grant.Enabled = enabled
	grant.Revision++
	if _, err := transaction.ExecContext(ctx, `UPDATE gatehouse_system_grants SET enabled = `+placeholder(1)+`, revision = `+placeholder(2)+` WHERE id = `+placeholder(3), grant.Enabled, grant.Revision, grant.Ref.Id); err != nil {
		return fmt.Errorf("update system grant: %w", err), nil
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Ref: model.ActivityEventRef{}, Event: "system_grant.update", ResourceKind: ActivityResourceKindSystemGrant, ResourceSystemGrant: &grant.Ref.Id}, []string{ActivityTopicSystemGrant(grant.Ref), ActivityTopicPrincipalGrant(grant.Principal, grant.Ref.Id)}); err != nil {
		return fmt.Errorf("append system grant update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil { return fmt.Errorf("commit system grant update: %w", err), nil }
	return nil, &grant
}
