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

const projectSecretDescriptionMaxSize = 4 * 1024

type ProjectSecretSummary struct {
	Ref             model.ProjectSecretRef
	AuthorPrincipal model.PrincipalRef
	AuthorName      *string
	Description     string
	CreatedAt       string
	UpdatedAt       string
}

type ProjectSecretDetail struct {
	Secret     model.ProjectSecret
	AuthorName *string
}

func normalizeProjectSecret(description, ciphertext *string) error {
	if description == nil || ciphertext == nil {
		return fmt.Errorf("project secret fields are required")
	}
	*description = strings.TrimSpace(*description)
	if *description == "" {
		return fmt.Errorf("project secret description must not be blank")
	}
	if len(*description) > projectSecretDescriptionMaxSize || *ciphertext == "" {
		return fmt.Errorf("project secret description or value is invalid")
	}
	return nil
}

func ProjectSecretAssociatedData(secret model.ProjectSecretRef) []byte {
	return []byte("gh=v1|project-secret=" + secret.Project.Workspace.Id + "|project=" + secret.Project.Id + "|id=" + secret.Id)
}

func (store *Store) ProjectSecretsGet(ctx context.Context, project model.ProjectRef, principal model.PrincipalRef) (error, []ProjectSecretSummary) {
	if err, available := store.ProjectGet(ctx, project, principal); err != nil {
		return err, nil
	} else if available == nil {
		return nil, []ProjectSecretSummary{}
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT secrets.id, secrets.author_principal, principals.name, secrets.description, secrets.created_at, secrets.updated_at
		FROM gatehouse_project_secrets AS secrets
		JOIN gatehouse_principals AS principals ON principals.id = secrets.author_principal
		WHERE secrets.workspace = `+placeholder(1)+` AND secrets.project = `+placeholder(2)+` AND secrets.enabled = TRUE
		ORDER BY secrets.created_at DESC, secrets.id DESC
	`, project.Workspace.Id, project.Id)
	if err != nil {
		return fmt.Errorf("get project secrets: %w", err), nil
	}
	defer rows.Close()
	secrets := []ProjectSecretSummary{}
	for rows.Next() {
		var secret ProjectSecretSummary
		var authorName sql.NullString
		secret.Ref.Project = project
		if err := rows.Scan(&secret.Ref.Id, &secret.AuthorPrincipal.Id, &authorName, &secret.Description, &secret.CreatedAt, &secret.UpdatedAt); err != nil {
			return fmt.Errorf("scan project secret: %w", err), nil
		}
		if authorName.Valid {
			secret.AuthorName = &authorName.String
		}
		secrets = append(secrets, secret)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project secrets: %w", err), nil
	}
	return nil, secrets
}

func (store *Store) ProjectSecretGet(ctx context.Context, secret model.ProjectSecretRef, principal model.PrincipalRef) (error, *ProjectSecretDetail) {
	if !typed_id.Valid(typed_id.ProjectSecret, secret.Id) {
		return fmt.Errorf("get project secret: ID is invalid"), nil
	}
	if err, project := store.ProjectGet(ctx, secret.Project, principal); err != nil {
		return err, nil
	} else if project == nil {
		return nil, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT secrets.author_principal, principals.name, secrets.description, secrets.ciphertext, secrets.enabled, secrets.created_at, secrets.updated_at
		FROM gatehouse_project_secrets AS secrets
		JOIN gatehouse_principals AS principals ON principals.id = secrets.author_principal
		WHERE secrets.workspace = `+placeholder(1)+` AND secrets.project = `+placeholder(2)+` AND secrets.id = `+placeholder(3)+` AND secrets.enabled = TRUE
	`, secret.Project.Workspace.Id, secret.Project.Id, secret.Id)
	detail := &ProjectSecretDetail{Secret: model.ProjectSecret{Ref: secret}}
	var authorName sql.NullString
	if err := row.Scan(&detail.Secret.AuthorPrincipal.Id, &authorName, &detail.Secret.Description, &detail.Secret.Ciphertext, &detail.Secret.Enabled, &detail.Secret.CreatedAt, &detail.Secret.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get project secret: %w", err), nil
	}
	if authorName.Valid {
		detail.AuthorName = &authorName.String
	}
	return nil, detail
}

func (store *Store) ProjectSecretCreate(ctx context.Context, secret model.ProjectSecret, principal model.PrincipalRef) (error, model.ProjectSecret) {
	if !typed_id.Valid(typed_id.ProjectSecret, secret.Ref.Id) || strings.TrimSpace(principal.Id) == "" {
		return fmt.Errorf("create project secret: IDs are invalid"), model.ProjectSecret{}
	}
	if err := normalizeProjectSecret(&secret.Description, &secret.Ciphertext); err != nil {
		return fmt.Errorf("create project secret: %w", err), model.ProjectSecret{}
	}
	if err, project := store.ProjectGet(ctx, secret.Ref.Project, principal); err != nil {
		return err, model.ProjectSecret{}
	} else if project == nil {
		return fmt.Errorf("create project secret: project is unavailable"), model.ProjectSecret{}
	}
	createdAt, err := typed_id.Timestamp(typed_id.ProjectSecret, secret.Ref.Id)
	if err != nil {
		return fmt.Errorf("create project secret: secret ID is invalid"), model.ProjectSecret{}
	}
	secret.AuthorPrincipal = principal
	secret.Enabled = true
	secret.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	secret.UpdatedAt = secret.CreatedAt
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project secret creation: %w", err), model.ProjectSecret{}
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_project_secrets (workspace, project, id, author_principal, description, ciphertext, enabled, created_at, updated_at)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, TRUE, `+placeholder(7)+`, `+placeholder(8)+`)
	`, secret.Ref.Project.Workspace.Id, secret.Ref.Project.Id, secret.Ref.Id, secret.AuthorPrincipal.Id, secret.Description, secret.Ciphertext, secret.CreatedAt, secret.UpdatedAt); err != nil {
		return fmt.Errorf("insert project secret: %w", err), model.ProjectSecret{}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{Workspace: secret.Ref.Project.Workspace},
		Event:        "project_secret.create",
		ResourceKind: ActivityResourceKindProject,
		Project:      &secret.Ref.Project,
	}, []string{ActivityTopicProject(secret.Ref.Project)}); err != nil {
		return fmt.Errorf("append project secret creation activity: %w", err), model.ProjectSecret{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project secret creation: %w", err), model.ProjectSecret{}
	}
	return nil, secret
}

func (store *Store) ProjectSecretDetailsSet(ctx context.Context, secret model.ProjectSecretRef, principal model.PrincipalRef, description, ciphertext *string) (error, *ProjectSecretDetail) {
	if err := normalizeProjectSecret(description, ciphertext); err != nil {
		return fmt.Errorf("update project secret: %w", err), nil
	}
	err, detail := store.ProjectSecretGet(ctx, secret, principal)
	if err != nil || detail == nil {
		return err, detail
	}
	updatedAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project secret update: %w", err), nil
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	result, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_project_secrets SET description = `+placeholder(1)+`, ciphertext = `+placeholder(2)+`, updated_at = `+placeholder(3)+`
		WHERE workspace = `+placeholder(4)+` AND project = `+placeholder(5)+` AND id = `+placeholder(6)+` AND enabled = TRUE
	`, *description, *ciphertext, updatedAt, secret.Project.Workspace.Id, secret.Project.Id, secret.Id)
	if err != nil {
		return fmt.Errorf("update project secret: %w", err), nil
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update project secret: %w", err), nil
	}
	if changed != 1 {
		return nil, nil
	}
	detail.Secret.Description, detail.Secret.Ciphertext, detail.Secret.UpdatedAt = *description, *ciphertext, updatedAt
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{Workspace: secret.Project.Workspace},
		Event:        "project_secret.update",
		ResourceKind: ActivityResourceKindProject,
		Project:      &secret.Project,
	}, []string{ActivityTopicProject(secret.Project)}); err != nil {
		return fmt.Errorf("append project secret update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project secret update: %w", err), nil
	}
	return nil, detail
}

func (store *Store) ProjectSecretRemove(ctx context.Context, secret model.ProjectSecretRef, principal model.PrincipalRef) (error, bool) {
	if !typed_id.Valid(typed_id.ProjectSecret, secret.Id) {
		return fmt.Errorf("remove project secret: ID is invalid"), false
	}
	if err, project := store.ProjectGet(ctx, secret.Project, principal); err != nil {
		return err, false
	} else if project == nil {
		return nil, false
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project secret removal: %w", err), false
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	result, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_project_secrets SET ciphertext = '', enabled = FALSE, updated_at = `+placeholder(1)+`
		WHERE workspace = `+placeholder(2)+` AND project = `+placeholder(3)+` AND id = `+placeholder(4)+` AND enabled = TRUE
	`, time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), secret.Project.Workspace.Id, secret.Project.Id, secret.Id)
	if err != nil {
		return fmt.Errorf("remove project secret: %w", err), false
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove project secret: %w", err), false
	}
	if changed != 1 {
		return nil, false
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{Workspace: secret.Project.Workspace},
		Event:        "project_secret.remove",
		ResourceKind: ActivityResourceKindProject,
		Project:      &secret.Project,
	}, []string{ActivityTopicProject(secret.Project)}); err != nil {
		return fmt.Errorf("append project secret removal activity: %w", err), false
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project secret removal: %w", err), false
	}
	return nil, true
}
