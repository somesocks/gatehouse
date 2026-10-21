package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gatehouse/identity"
	"gatehouse/typed_id"
)

var ErrPasswordLinkUnavailable = errors.New("password link is no longer available")
var ErrPasswordLinkExpired = errors.New("password link has expired")

// PasswordSetIdentityGet returns only enabled local logins under enabled owners.
func (store *Store) PasswordSetIdentityGet(ctx context.Context, principal, id string) (error, *SystemPrincipalIdentity) {
	if !typed_id.Valid(typed_id.Principal, principal) || !typed_id.Valid(typed_id.Identity, id) {
		return nil, nil
	}
	p := keychainPlaceholder(store.kind)
	var login SystemPrincipalIdentity
	err := store.QueryRowContext(ctx, `SELECT identities.id, identities.key, identities.enabled, identities.revision
		FROM gatehouse_identities AS identities JOIN gatehouse_principals AS principals ON principals.id = identities.principal_id
		WHERE identities.id = `+p(1)+` AND identities.principal_id = `+p(2)+` AND identities.enabled = TRUE AND principals.enabled = TRUE`, id, principal).Scan(&login.ID, &login.Key, &login.Enabled, &login.Revision)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return err, nil
	}
	if !strings.HasPrefix(login.Key, "gatehouse:") {
		return nil, nil
	}
	return nil, &login
}

// PasswordSet consumes a capability revision and changes the password in the
// same transaction. The conditional UPDATE prevents concurrent redemption.
func (store *Store) PasswordSet(ctx context.Context, principal, id string, revision int, expiresAt time.Time, password []byte) error {
	if !typed_id.Valid(typed_id.Principal, principal) || !typed_id.Valid(typed_id.Identity, id) || revision < 0 {
		return ErrPasswordLinkUnavailable
	}
	value, err := identity.NewPasswordVerifier(password, store.Crypto())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPrincipalInput, err)
	}
	encoded, err := json.Marshal([]map[string]string{{"kind": "password", "password_verifier": value}})
	if err != nil {
		return err
	}
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Hashing can take time: enforce expiration again at mutation time.
	if !time.Now().Before(expiresAt) {
		return ErrPasswordLinkExpired
	}
	p := keychainPlaceholder(store.kind)
	verifierSQL := p(1)
	if store.kind == "postgres" {
		verifierSQL += "::jsonb"
	}
	result, err := tx.ExecContext(ctx, `UPDATE gatehouse_identities SET verifiers = `+verifierSQL+`, revision = revision + 1
		WHERE id = `+p(2)+` AND principal_id = `+p(3)+` AND revision = `+p(4)+` AND enabled = TRUE
		AND substr(key, 1, 10) = 'gatehouse:'
		AND EXISTS (SELECT 1 FROM gatehouse_principals WHERE id = `+p(5)+` AND enabled = TRUE)`, string(encoded), id, principal, revision, principal)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrRevisionConflict
	}
	// A database lock may have delayed the UPDATE past the deadline. Roll back
	// rather than committing a password set by an expired capability.
	if !time.Now().Before(expiresAt) {
		return ErrPasswordLinkExpired
	}
	if err := store.principalIdentityActivity(ctx, tx, principal, id, "identity.update"); err != nil {
		return err
	}
	return tx.Commit()
}
