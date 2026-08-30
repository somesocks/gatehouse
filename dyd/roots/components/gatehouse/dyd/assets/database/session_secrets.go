package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gatehouse/model"
	"gatehouse/typed_id"
)

const sessionSecretDescriptionMaxSize = 4 * 1024

type SessionSecretSummary struct {
	Ref             model.SessionSecretRef
	AuthorPrincipal model.PrincipalRef
	AuthorName      *string
	Description     string
	CreatedAt       string
	UpdatedAt       string
}

type SessionSecretDetail struct {
	Secret     model.SessionSecret
	AuthorName *string
}

func normalizeSessionSecret(description, ciphertext *string) error {
	if description == nil || ciphertext == nil {
		return fmt.Errorf("session secret fields are required")
	}
	*description = strings.TrimSpace(*description)
	if *description == "" {
		return fmt.Errorf("session secret description must not be blank")
	}
	if len(*description) > sessionSecretDescriptionMaxSize || *ciphertext == "" {
		return fmt.Errorf("session secret description or value is invalid")
	}
	return nil
}

func SessionSecretAssociatedData(secret model.SessionSecretRef) []byte {
	return []byte("gh=v1|session-secret=" + secret.Session.Workspace.Id + "|session=" + secret.Session.Id + "|id=" + secret.Id)
}

func (store *Store) SessionSecretsGet(ctx context.Context, session model.SessionRef, principal model.PrincipalRef) (error, []SessionSecretSummary) {
	if err, available := store.SessionGet(ctx, session, principal); err != nil {
		return err, nil
	} else if available == nil {
		return nil, []SessionSecretSummary{}
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT secrets.id, secrets.author_principal, principals.name, secrets.description, secrets.created_at, secrets.updated_at
		FROM gatehouse_session_secrets AS secrets
		JOIN gatehouse_principals AS principals ON principals.id = secrets.author_principal
		WHERE secrets.workspace = `+placeholder(1)+` AND secrets.session = `+placeholder(2)+` AND secrets.enabled = TRUE
		ORDER BY secrets.created_at DESC, secrets.id DESC
	`, session.Workspace.Id, session.Id)
	if err != nil {
		return fmt.Errorf("get session secrets: %w", err), nil
	}
	defer rows.Close()
	secrets := []SessionSecretSummary{}
	for rows.Next() {
		var secret SessionSecretSummary
		var authorName sql.NullString
		secret.Ref.Session = session
		if err := rows.Scan(&secret.Ref.Id, &secret.AuthorPrincipal.Id, &authorName, &secret.Description, &secret.CreatedAt, &secret.UpdatedAt); err != nil {
			return fmt.Errorf("scan session secret: %w", err), nil
		}
		if authorName.Valid {
			secret.AuthorName = &authorName.String
		}
		secrets = append(secrets, secret)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate session secrets: %w", err), nil
	}
	return nil, secrets
}

func (store *Store) SessionSecretGet(ctx context.Context, secret model.SessionSecretRef, principal model.PrincipalRef) (error, *SessionSecretDetail) {
	if !typed_id.Valid(typed_id.SessionSecret, secret.Id) {
		return fmt.Errorf("get session secret: ID is invalid"), nil
	}
	if err, session := store.SessionGet(ctx, secret.Session, principal); err != nil {
		return err, nil
	} else if session == nil {
		return nil, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT secrets.author_principal, principals.name, secrets.description, secrets.ciphertext, secrets.enabled, secrets.created_at, secrets.updated_at
		FROM gatehouse_session_secrets AS secrets
		JOIN gatehouse_principals AS principals ON principals.id = secrets.author_principal
		WHERE secrets.workspace = `+placeholder(1)+` AND secrets.session = `+placeholder(2)+` AND secrets.id = `+placeholder(3)+` AND secrets.enabled = TRUE
	`, secret.Session.Workspace.Id, secret.Session.Id, secret.Id)
	detail := &SessionSecretDetail{Secret: model.SessionSecret{Ref: secret}}
	var authorName sql.NullString
	if err := row.Scan(&detail.Secret.AuthorPrincipal.Id, &authorName, &detail.Secret.Description, &detail.Secret.Ciphertext, &detail.Secret.Enabled, &detail.Secret.CreatedAt, &detail.Secret.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get session secret: %w", err), nil
	}
	if authorName.Valid {
		detail.AuthorName = &authorName.String
	}
	return nil, detail
}

func (store *Store) SessionSecretCreate(ctx context.Context, secret model.SessionSecret, principal model.PrincipalRef) (error, model.SessionSecret) {
	if !typed_id.Valid(typed_id.SessionSecret, secret.Ref.Id) || strings.TrimSpace(principal.Id) == "" {
		return fmt.Errorf("create session secret: IDs are invalid"), model.SessionSecret{}
	}
	if err := normalizeSessionSecret(&secret.Description, &secret.Ciphertext); err != nil {
		return fmt.Errorf("create session secret: %w", err), model.SessionSecret{}
	}
	if err, session := store.SessionGet(ctx, secret.Ref.Session, principal); err != nil {
		return err, model.SessionSecret{}
	} else if session == nil {
		return fmt.Errorf("create session secret: session is unavailable"), model.SessionSecret{}
	}
	createdAt, err := typed_id.Timestamp(typed_id.SessionSecret, secret.Ref.Id)
	if err != nil {
		return fmt.Errorf("create session secret: secret ID is invalid"), model.SessionSecret{}
	}
	secret.AuthorPrincipal = principal
	secret.Enabled = true
	secret.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	secret.UpdatedAt = secret.CreatedAt
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session secret creation: %w", err), model.SessionSecret{}
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_session_secrets (workspace, session, id, author_principal, description, ciphertext, enabled, created_at, updated_at)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, TRUE, `+placeholder(7)+`, `+placeholder(8)+`)
	`, secret.Ref.Session.Workspace.Id, secret.Ref.Session.Id, secret.Ref.Id, secret.AuthorPrincipal.Id, secret.Description, secret.Ciphertext, secret.CreatedAt, secret.UpdatedAt); err != nil {
		return fmt.Errorf("insert session secret: %w", err), model.SessionSecret{}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{Workspace: secret.Ref.Session.Workspace},
		Event:        "session_secret.create",
		ResourceKind: ActivityResourceKindSession,
		Session:      &secret.Ref.Session,
	}, []string{ActivityTopicSession(secret.Ref.Session)}); err != nil {
		return fmt.Errorf("append session secret creation activity: %w", err), model.SessionSecret{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session secret creation: %w", err), model.SessionSecret{}
	}
	return nil, secret
}

func (store *Store) SessionSecretDetailsSet(ctx context.Context, secret model.SessionSecretRef, principal model.PrincipalRef, description, ciphertext *string) (error, *SessionSecretDetail) {
	if err := normalizeSessionSecret(description, ciphertext); err != nil {
		return fmt.Errorf("update session secret: %w", err), nil
	}
	err, detail := store.SessionSecretGet(ctx, secret, principal)
	if err != nil || detail == nil {
		return err, detail
	}
	updatedAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session secret update: %w", err), nil
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	result, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_session_secrets SET description = `+placeholder(1)+`, ciphertext = `+placeholder(2)+`, updated_at = `+placeholder(3)+`
		WHERE workspace = `+placeholder(4)+` AND session = `+placeholder(5)+` AND id = `+placeholder(6)+` AND enabled = TRUE
	`, *description, *ciphertext, updatedAt, secret.Session.Workspace.Id, secret.Session.Id, secret.Id)
	if err != nil {
		return fmt.Errorf("update session secret: %w", err), nil
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update session secret: %w", err), nil
	}
	if changed != 1 {
		return nil, nil
	}
	detail.Secret.Description, detail.Secret.Ciphertext, detail.Secret.UpdatedAt = *description, *ciphertext, updatedAt
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{Workspace: secret.Session.Workspace},
		Event:        "session_secret.update",
		ResourceKind: ActivityResourceKindSession,
		Session:      &secret.Session,
	}, []string{ActivityTopicSession(secret.Session)}); err != nil {
		return fmt.Errorf("append session secret update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session secret update: %w", err), nil
	}
	return nil, detail
}

func (store *Store) SessionSecretRemove(ctx context.Context, secret model.SessionSecretRef, principal model.PrincipalRef) (error, bool) {
	if !typed_id.Valid(typed_id.SessionSecret, secret.Id) {
		return fmt.Errorf("remove session secret: ID is invalid"), false
	}
	if err, session := store.SessionGet(ctx, secret.Session, principal); err != nil {
		return err, false
	} else if session == nil {
		return nil, false
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session secret removal: %w", err), false
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	result, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_session_secrets SET ciphertext = '', enabled = FALSE, updated_at = `+placeholder(1)+`
		WHERE workspace = `+placeholder(2)+` AND session = `+placeholder(3)+` AND id = `+placeholder(4)+` AND enabled = TRUE
	`, time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), secret.Session.Workspace.Id, secret.Session.Id, secret.Id)
	if err != nil {
		return fmt.Errorf("remove session secret: %w", err), false
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove session secret: %w", err), false
	}
	if changed != 1 {
		return nil, false
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{Workspace: secret.Session.Workspace},
		Event:        "session_secret.remove",
		ResourceKind: ActivityResourceKindSession,
		Session:      &secret.Session,
	}, []string{ActivityTopicSession(secret.Session)}); err != nil {
		return fmt.Errorf("append session secret removal activity: %w", err), false
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session secret removal: %w", err), false
	}
	return nil, true
}
