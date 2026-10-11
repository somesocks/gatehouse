package database

import (
	"context"
	"database/sql"
	"fmt"
	"gatehouse/model"
	"gatehouse/typed_id"
	"strings"
)

func (store *Store) SystemGrantsSearch(ctx context.Context, name, cursor, status string, limit int) (error, []model.SystemGrant, []SystemPrincipal, string) {
	if limit < 1 || limit > 100 || len(name) > 256 || (cursor != "" && !typed_id.Valid(typed_id.SystemGrant, cursor)) || (status != "" && status != "enabled" && status != "disabled") {
		return fmt.Errorf("invalid system grant search"), nil, nil, ""
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(name)) + "%"
	p := keychainPlaceholder(store.kind)
	query := `SELECT grants.id, grants.principal, grants.enabled, grants.revision, principals.alias, principals.name, principals.enabled, principals.revision FROM gatehouse_system_grants AS grants JOIN gatehouse_principals AS principals ON principals.id = grants.principal WHERE (LOWER(COALESCE(principals.alias,'')) LIKE LOWER(` + p(1) + `) ESCAPE '\' OR LOWER(COALESCE(principals.name,'')) LIKE LOWER(` + p(2) + `) ESCAPE '\' OR LOWER(grants.principal) LIKE LOWER(` + p(3) + `) ESCAPE '\') AND (` + p(4) + ` = '' OR grants.id < ` + p(5) + `)`
	args := []any{pattern, pattern, pattern, cursor, cursor}
	if status != "" {
		query += ` AND grants.enabled = ` + p(6)
		args = append(args, status == "enabled")
	}
	query += ` ORDER BY grants.id DESC LIMIT ` + p(len(args)+1)
	args = append(args, limit+1)
	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return err, nil, nil, ""
	}
	defer rows.Close()
	grants := []model.SystemGrant{}
	principals := []SystemPrincipal{}
	for rows.Next() {
		var grant model.SystemGrant
		var principal SystemPrincipal
		var alias, name sql.NullString
		if err := rows.Scan(&grant.Ref.Id, &grant.Principal.Id, &grant.Enabled, &grant.Revision, &alias, &name, &principal.Enabled, &principal.Revision); err != nil {
			return err, nil, nil, ""
		}
		principal.ID = grant.Principal.Id
		if alias.Valid {
			principal.Alias = &alias.String
		}
		if name.Valid {
			principal.Name = &name.String
		}
		principal.Identities = []SystemPrincipalIdentity{}
		grants = append(grants, grant)
		principals = append(principals, principal)
	}
	if err := rows.Err(); err != nil {
		return err, nil, nil, ""
	}
	if len(grants) <= limit {
		return nil, grants, principals, ""
	}
	return nil, grants[:limit], principals[:limit], grants[limit-1].Ref.Id
}
