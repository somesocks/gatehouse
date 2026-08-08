package database

import (
	"context"
	"fmt"
	"strings"

	"gatehouse/config"
	"gatehouse/model"
)

func (store *Store) KeychainsInsert(ctx context.Context, keychains []model.Keychain) error {
	if len(keychains) == 0 {
		return nil
	}

	arguments := make([]any, 0, len(keychains)*5)
	values := make([]string, 0, len(keychains))
	placeholder := keychainPlaceholder(store.kind)
	for keychainIndex, keychain := range keychains {
		firstArgument := keychainIndex*5 + 1
		values = append(values, fmt.Sprintf("(%s, %s, %s, %s, %s)",
			placeholder(firstArgument),
			placeholder(firstArgument+1),
			placeholder(firstArgument+2),
			placeholder(firstArgument+3),
			placeholder(firstArgument+4),
		))
		arguments = append(arguments,
			keychain.Ref.Id,
			keychain.Ref.Version,
			keychain.KekKdf,
			keychain.Key,
			keychain.Enabled,
		)
	}

	_, err := store.ExecContext(ctx, `
		INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled)
		VALUES `+strings.Join(values, ", ")+`
		ON CONFLICT (id, version) DO NOTHING
	`, arguments...)
	if err != nil {
		return fmt.Errorf("insert keychains: %w", err)
	}
	return nil
}

func (store *Store) KeychainsGet(ctx context.Context, references []model.KeychainRef) (error, []model.Keychain) {
	if len(references) == 0 {
		return nil, []model.Keychain{}
	}

	arguments := make([]any, 0, len(references)*2)
	values := make([]string, 0, len(references))
	placeholder := keychainPlaceholder(store.kind)
	for referenceIndex, reference := range references {
		firstArgument := referenceIndex*2 + 1
		values = append(values, fmt.Sprintf("(%s, %s)", placeholder(firstArgument), placeholder(firstArgument+1)))
		arguments = append(arguments, reference.Id, reference.Version)
	}

	rows, err := store.QueryContext(ctx, `
		SELECT id, version, kek_kdf, key, enabled
		FROM gatehouse_keychains
		WHERE (id, version) IN (`+strings.Join(values, ", ")+`)
		ORDER BY id, version
	`, arguments...)
	if err != nil {
		return fmt.Errorf("get keychains: %w", err), nil
	}
	return readKeychains(rows)
}

func (store *Store) KeychainsGetCurrent(ctx context.Context, ids []string) (error, []model.Keychain) {
	if len(ids) == 0 {
		return nil, []model.Keychain{}
	}

	arguments := make([]any, len(ids))
	values := make([]string, len(ids))
	placeholder := keychainPlaceholder(store.kind)
	for idIndex, id := range ids {
		arguments[idIndex] = id
		values[idIndex] = placeholder(idIndex + 1)
	}

	rows, err := store.QueryContext(ctx, `
		SELECT id, version, kek_kdf, key, enabled
		FROM gatehouse_keychains AS keychains
		WHERE enabled = TRUE
			AND id IN (`+strings.Join(values, ", ")+`)
			AND version = (
				SELECT MAX(version)
				FROM gatehouse_keychains AS candidates
				WHERE candidates.id = keychains.id AND candidates.enabled = TRUE
			)
		ORDER BY id
	`, arguments...)
	if err != nil {
		return fmt.Errorf("get current keychains: %w", err), nil
	}
	return readKeychains(rows)
}

func keychainPlaceholder(kind config.DatabaseKind) func(int) string {
	switch kind {
	case config.DatabaseKindSQLite:
		return func(int) string { return "?" }
	case config.DatabaseKindPostgres:
		return func(index int) string { return fmt.Sprintf("$%d", index) }
	default:
		panic(fmt.Sprintf("unsupported database kind %q", kind))
	}
}

func readKeychains(rows interface {
	Close() error
	Err() error
	Next() bool
	Scan(...any) error
}) (error, []model.Keychain) {
	defer rows.Close()

	keychains := []model.Keychain{}
	for rows.Next() {
		var keychain model.Keychain
		if err := rows.Scan(
			&keychain.Ref.Id,
			&keychain.Ref.Version,
			&keychain.KekKdf,
			&keychain.Key,
			&keychain.Enabled,
		); err != nil {
			return fmt.Errorf("scan keychain: %w", err), nil
		}
		keychains = append(keychains, keychain)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate keychains: %w", err), nil
	}
	return nil, keychains
}
