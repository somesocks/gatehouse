package database

import (
	"context"
	"fmt"
	"gatehouse/typed_id"
	"strings"
)

func (store *Store) SystemStorageProvidersSearch(ctx context.Context, name, cursor, status string, limit int) (error, []SystemStorageProvider, string) {
	if limit < 1 || limit > 100 || len(name) > 256 || (cursor != "" && !typed_id.Valid(typed_id.StorageProvider, cursor)) || (status != "" && status != "enabled" && status != "disabled") {
		return fmt.Errorf("invalid storage provider search"), nil, ""
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(name)) + "%"
	p := keychainPlaceholder(store.kind)
	query := `SELECT id, alias, revision, protocol, endpoint, region, bucket, access_key_id, keychain_id, keychain_version, secret_access_key IS NOT NULL, enabled FROM gatehouse_storage_providers WHERE (LOWER(alias) LIKE LOWER(` + p(1) + `) ESCAPE '\' OR LOWER(id) LIKE LOWER(` + p(2) + `) ESCAPE '\' OR LOWER(protocol) LIKE LOWER(` + p(3) + `) ESCAPE '\' OR LOWER(COALESCE(bucket,'')) LIKE LOWER(` + p(4) + `) ESCAPE '\') AND (` + p(5) + ` = '' OR id < ` + p(6) + `)`
	args := []any{pattern, pattern, pattern, pattern, cursor, cursor}
	if status != "" {
		query += ` AND enabled = ` + p(7)
		args = append(args, status == "enabled")
	}
	query += ` ORDER BY id DESC LIMIT ` + p(len(args)+1)
	args = append(args, limit+1)
	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return err, nil, ""
	}
	defer rows.Close()
	providers := []SystemStorageProvider{}
	for rows.Next() {
		provider, err := scanSystemStorageProvider(rows)
		if err != nil {
			return err, nil, ""
		}
		providers = append(providers, provider)
	}
	if err := rows.Err(); err != nil {
		return err, nil, ""
	}
	if len(providers) <= limit {
		return nil, providers, ""
	}
	return nil, providers[:limit], providers[limit-1].ID
}
