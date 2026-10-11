package database

import (
	"context"
	"database/sql"
	"fmt"
	"gatehouse/typed_id"
	"strings"
)

func (store *Store) SystemPrincipalsSearch(ctx context.Context, name, cursor, status string, limit int) (error, []SystemPrincipal, string) {
	if limit < 1 || limit > 100 || len(name) > 256 || (cursor != "" && !typed_id.Valid(typed_id.Principal, cursor)) || (status != "" && status != "enabled" && status != "disabled") {
		return fmt.Errorf("invalid principal search"), nil, ""
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(name)) + "%"
	p := keychainPlaceholder(store.kind)
	query := `SELECT id, alias, name, enabled, revision, (SELECT COUNT(*) FROM gatehouse_identities WHERE principal_id = principals.id) FROM gatehouse_principals AS principals WHERE (LOWER(COALESCE(alias,'')) LIKE LOWER(` + p(1) + `) ESCAPE '\' OR LOWER(COALESCE(name,'')) LIKE LOWER(` + p(2) + `) ESCAPE '\' OR LOWER(id) LIKE LOWER(` + p(3) + `) ESCAPE '\') AND (` + p(4) + ` = '' OR id < ` + p(5) + `)`
	args := []any{pattern, pattern, pattern, cursor, cursor}
	if status != "" {
		query += ` AND enabled = ` + p(6)
		args = append(args, status == "enabled")
	}
	query += ` ORDER BY id DESC LIMIT ` + p(len(args)+1)
	args = append(args, limit+1)
	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return err, nil, ""
	}
	defer rows.Close()
	principals := []SystemPrincipal{}
	for rows.Next() {
		var principal SystemPrincipal
		var alias, name sql.NullString
		if err := rows.Scan(&principal.ID, &alias, &name, &principal.Enabled, &principal.Revision, &principal.IdentityCount); err != nil {
			return err, nil, ""
		}
		if alias.Valid {
			principal.Alias = &alias.String
		}
		if name.Valid {
			principal.Name = &name.String
		}
		principal.Identities = []SystemPrincipalIdentity{}
		principals = append(principals, principal)
	}
	if err := rows.Err(); err != nil {
		return err, nil, ""
	}
	if len(principals) <= limit {
		return nil, principals, ""
	}
	return nil, principals[:limit], principals[limit-1].ID
}
