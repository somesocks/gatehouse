package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"gatehouse/identity"
	"gatehouse/model"
	"gatehouse/typed_id"
)

var (
	ErrPrincipalInput         = errors.New("invalid principal input")
	ErrPrincipalAliasExists   = errors.New("principal alias is already in use")
	ErrIdentityUsernameExists = errors.New("username is already in use")
)

type SystemPrincipalCreateInput struct {
	Alias   *string
	Name    *string
	Enabled bool
}

type SystemPrincipalIdentityInput struct {
	Username *string
	Password *string
	Enabled  *bool
}

func (store *Store) SystemPrincipalCreate(ctx context.Context, input SystemPrincipalCreateInput) (error, *SystemPrincipal) {
	input.Alias = normalizeOptionalString(input.Alias)
	input.Name = normalizeOptionalString(input.Name)
	if input.Alias != nil && (len(*input.Alias) > 256 || !administrationAlias.MatchString(*input.Alias)) {
		return fmt.Errorf("%w: alias must start with a lowercase letter and contain only lowercase letters, numbers, underscores, or hyphens", ErrPrincipalInput), nil
	}
	if input.Name != nil && (len(*input.Name) > 256 || strings.ContainsRune(*input.Name, 0)) {
		return fmt.Errorf("%w: display name must be at most 256 bytes and must not contain NUL", ErrPrincipalInput), nil
	}
	id, err := typed_id.New(typed_id.Principal)
	if err != nil {
		return err, nil
	}
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return err, nil
	}
	defer tx.Rollback()
	p := keychainPlaceholder(store.kind)
	var inserted string
	err = tx.QueryRowContext(ctx, `INSERT INTO gatehouse_principals (id, alias, name, enabled, revision) VALUES (`+p(1)+`, `+p(2)+`, `+p(3)+`, `+p(4)+`, 1) ON CONFLICT (alias) DO NOTHING RETURNING id`, id, optionalStringValue(input.Alias), optionalStringValue(input.Name), input.Enabled).Scan(&inserted)
	if err == sql.ErrNoRows {
		return ErrPrincipalAliasExists, nil
	}
	if err != nil {
		return err, nil
	}
	if err, _ := store.ActivityEventAppend(ctx, tx, model.ActivityEvent{Event: "principal.create", ResourceKind: ActivityResourceKindPrincipal, ResourcePrincipal: &id}, []string{id}); err != nil {
		return err, nil
	}
	if err := tx.Commit(); err != nil {
		return err, nil
	}
	return store.SystemPrincipalGet(ctx, id)
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	text := strings.TrimSpace(*value)
	if text == "" {
		return nil
	}
	return &text
}

func (store *Store) principalIdentityValues(input SystemPrincipalIdentityInput, creating bool) (*string, *string, error) {
	var key, verifiers *string
	if creating && (input.Username == nil || input.Password == nil) {
		return nil, nil, fmt.Errorf("%w: username and password are required", ErrPrincipalInput)
	}
	if input.Username != nil {
		username := *input.Username
		if username == "" || len(username) > 256 || strings.ContainsRune(username, ':') || strings.IndexFunc(username, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
			return nil, nil, fmt.Errorf("%w: username must be 1–256 bytes without whitespace, control characters, or colons", ErrPrincipalInput)
		}
		value := "gatehouse:" + username
		key = &value
	}
	if input.Password != nil {
		password := []byte(*input.Password)
		defer clear(password)
		value, err := identity.NewPasswordVerifier(password, store.Crypto())
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %v", ErrPrincipalInput, err)
		}
		encoded, err := json.Marshal([]map[string]string{{"kind": "password", "password_verifier": value}})
		if err != nil {
			return nil, nil, err
		}
		text := string(encoded)
		verifiers = &text
	}
	return key, verifiers, nil
}

func (store *Store) SystemPrincipalIdentityCreate(ctx context.Context, principal string, input SystemPrincipalIdentityInput) (error, *SystemPrincipal) {
	if !typed_id.Valid(typed_id.Principal, principal) {
		return nil, nil
	}
	key, verifiers, err := store.principalIdentityValues(input, true)
	if err != nil {
		return err, nil
	}
	id, err := typed_id.New(typed_id.Identity)
	if err != nil {
		return err, nil
	}
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return err, nil
	}
	defer tx.Rollback()
	p := keychainPlaceholder(store.kind)
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gatehouse_principals WHERE id = `+p(1)+`)`, principal).Scan(&exists); err != nil {
		return err, nil
	}
	if !exists {
		return nil, nil
	}
	enabled := input.Enabled == nil || *input.Enabled
	var inserted string
	err = tx.QueryRowContext(ctx, `INSERT INTO gatehouse_identities (id, key, principal_id, verifiers, enabled, revision) VALUES (`+p(1)+`, `+p(2)+`, `+p(3)+`, `+p(4)+`, `+p(5)+`, 1) ON CONFLICT (key) DO NOTHING RETURNING id`, id, *key, principal, *verifiers, enabled).Scan(&inserted)
	if err == sql.ErrNoRows {
		return ErrIdentityUsernameExists, nil
	}
	if err != nil {
		return err, nil
	}
	if err := store.principalIdentityActivity(ctx, tx, principal, id, "identity.create"); err != nil {
		return err, nil
	}
	if err := tx.Commit(); err != nil {
		return err, nil
	}
	return store.SystemPrincipalGet(ctx, principal)
}

func (store *Store) SystemPrincipalIdentityUpdate(ctx context.Context, principal, id string, revision int, input SystemPrincipalIdentityInput) (error, *SystemPrincipal) {
	if !typed_id.Valid(typed_id.Principal, principal) || !typed_id.Valid(typed_id.Identity, id) {
		return nil, nil
	}
	if revision < 0 || (input.Username == nil && input.Password == nil && input.Enabled == nil) {
		return fmt.Errorf("%w: expected revision and an update are required", ErrPrincipalInput), nil
	}
	key, verifiers, err := store.principalIdentityValues(input, false)
	if err != nil {
		return err, nil
	}
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return err, nil
	}
	defer tx.Rollback()
	p := keychainPlaceholder(store.kind)
	var currentKey string
	var currentRevision int
	var currentEnabled bool
	err = tx.QueryRowContext(ctx, `SELECT key, enabled, revision FROM gatehouse_identities WHERE id = `+p(1)+` AND principal_id = `+p(2), id, principal).Scan(&currentKey, &currentEnabled, &currentRevision)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return err, nil
	}
	if currentRevision != revision {
		return ErrRevisionConflict, nil
	}
	if !strings.HasPrefix(currentKey, "gatehouse:") {
		return fmt.Errorf("%w: only username/password identities can be edited", ErrPrincipalInput), nil
	}
	if key == nil {
		key = &currentKey
	}
	enabled := currentEnabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	if *key != currentKey || enabled != currentEnabled || verifiers != nil {
		// Preserve live verifier values when a password was not supplied.
		verifierSQL := p(3)
		if store.kind == "postgres" {
			verifierSQL += "::jsonb"
		}
		result, err := tx.ExecContext(ctx, `UPDATE gatehouse_identities SET key = `+p(1)+`, enabled = `+p(2)+`, revision = revision + 1, verifiers = COALESCE(`+verifierSQL+`, verifiers) WHERE id = `+p(4)+` AND principal_id = `+p(5)+` AND revision = `+p(6), *key, enabled, optionalStringValue(verifiers), id, principal, revision)
		if uniquePrincipalConstraint(err) {
			return ErrIdentityUsernameExists, nil
		}
		if err != nil {
			return err, nil
		}
		if count, err := result.RowsAffected(); err != nil {
			return err, nil
		} else if count != 1 {
			return ErrRevisionConflict, nil
		}
		if err := store.principalIdentityActivity(ctx, tx, principal, id, "identity.update"); err != nil {
			return err, nil
		}
	}
	if err := tx.Commit(); err != nil {
		return err, nil
	}
	return store.SystemPrincipalGet(ctx, principal)
}

func (store *Store) SystemPrincipalIdentityRemove(ctx context.Context, principal, id string, revision int) (error, *SystemPrincipal) {
	if !typed_id.Valid(typed_id.Principal, principal) || !typed_id.Valid(typed_id.Identity, id) {
		return nil, nil
	}
	if revision < 0 {
		return fmt.Errorf("%w: expected revision is required", ErrPrincipalInput), nil
	}
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return err, nil
	}
	defer tx.Rollback()
	p := keychainPlaceholder(store.kind)
	var current int
	err = tx.QueryRowContext(ctx, `SELECT revision FROM gatehouse_identities WHERE id = `+p(1)+` AND principal_id = `+p(2), id, principal).Scan(&current)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return err, nil
	}
	if current != revision {
		return ErrRevisionConflict, nil
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM gatehouse_identities WHERE id = `+p(1)+` AND principal_id = `+p(2)+` AND revision = `+p(3), id, principal, revision)
	if err != nil {
		return err, nil
	}
	if count, err := result.RowsAffected(); err != nil {
		return err, nil
	} else if count != 1 {
		return ErrRevisionConflict, nil
	}
	if err := store.principalIdentityActivity(ctx, tx, principal, id, "identity.remove"); err != nil {
		return err, nil
	}
	if err := tx.Commit(); err != nil {
		return err, nil
	}
	return store.SystemPrincipalGet(ctx, principal)
}

func (store *Store) principalIdentityActivity(ctx context.Context, tx *sql.Tx, principal, id, event string) error {
	err, _ := store.ActivityEventAppend(ctx, tx, model.ActivityEvent{Event: event, ResourceKind: ActivityResourceKindPrincipal, ResourcePrincipal: &principal, DetailIdentity: &id}, []string{principal + "/" + id})
	return err
}

func uniquePrincipalConstraint(err error) bool {
	var postgres interface{ SQLState() string }
	var sqlite interface{ Code() int }
	return (errors.As(err, &postgres) && postgres.SQLState() == "23505") || (errors.As(err, &sqlite) && sqlite.Code() == 2067)
}
