package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"gatehouse/model"
	"gatehouse/typed_id"
)

var (
	ErrGroupSecretInput    = errors.New("invalid group secret")
	ErrGroupSecretNotFound = errors.New("group secret is unavailable")
)

type GroupSecretSummary struct {
	Ref             model.GroupSecretRef
	AuthorPrincipal model.PrincipalRef
	AuthorName      *string
	GroupName       *string
	Description     string
	Revision        int
	CreatedAt       string
	UpdatedAt       string
}

type GroupSecretDetail struct {
	Secret     model.GroupSecret
	AuthorName *string
	GroupName  *string
}

func GroupSecretAssociatedData(ref model.GroupSecretRef) []byte {
	return []byte("gh=v1|group-secret=" + ref.Workspace.Id + "|group=" + ref.Group.Id + "|id=" + ref.Id)
}

func groupSecretRefValid(ref model.GroupSecretRef) bool {
	return typed_id.Valid(typed_id.Workspace, ref.Workspace.Id) && typed_id.Valid(typed_id.Group, ref.Group.Id) && typed_id.Valid(typed_id.GroupSecret, ref.Id)
}

func normalizeGroupSecret(description *string, ciphertext *string) error {
	if description != nil {
		*description = strings.TrimSpace(*description)
		if *description == "" || len(*description) > 4096 {
			return ErrGroupSecretInput
		}
	}
	if ciphertext != nil && *ciphertext == "" {
		return ErrGroupSecretInput
	}
	return nil
}

func groupSecretPrincipalAccessSQL(workspace, group, principal string) string {
	return workspacePrincipalAdmissionSQL(workspace, principal) + ` AND EXISTS (
		SELECT 1 FROM gatehouse_groups visible_group JOIN gatehouse_group_members member
		ON member.workspace_id = visible_group.workspace_id AND member.group_id = visible_group.id
		WHERE visible_group.workspace_id = ` + workspace + ` AND visible_group.id = ` + group + ` AND visible_group.enabled = TRUE
		AND member.principal_id = ` + principal + ` AND member.enabled = TRUE)`
}

const groupSecretColumns = `secrets.workspace, secrets."group", secrets.id, secrets.author_principal, principals.name,
	groups.name, secrets.description, secrets.revision, secrets.created_at, secrets.updated_at`
const groupSecretFrom = ` FROM gatehouse_group_secrets secrets
	JOIN gatehouse_groups groups ON groups.workspace_id = secrets.workspace AND groups.id = secrets."group" AND groups.enabled = TRUE
	JOIN gatehouse_principals principals ON principals.id = secrets.author_principal `

func scanGroupSecret(row interface{ Scan(...any) error }, value bool) (GroupSecretDetail, error) {
	detail := GroupSecretDetail{Secret: model.GroupSecret{Enabled: true}}
	var authorName, groupName sql.NullString
	s := &detail.Secret
	values := []any{&s.Ref.Workspace.Id, &s.Ref.Group.Id, &s.Ref.Id, &s.AuthorPrincipal.Id, &authorName, &groupName, &s.Description, &s.Revision, &s.CreatedAt, &s.UpdatedAt}
	if value {
		values = append(values, &s.Ciphertext, &s.Enabled)
	}
	if err := row.Scan(values...); err != nil {
		return GroupSecretDetail{}, err
	}
	if authorName.Valid {
		detail.AuthorName = &authorName.String
	}
	if groupName.Valid {
		detail.GroupName = &groupName.String
	}
	return detail, nil
}

func groupSecretSummary(detail GroupSecretDetail) GroupSecretSummary {
	s := detail.Secret
	return GroupSecretSummary{Ref: s.Ref, AuthorPrincipal: s.AuthorPrincipal, AuthorName: detail.AuthorName, GroupName: detail.GroupName, Description: s.Description, Revision: s.Revision, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
}

func (store *Store) groupSecretsSearch(ctx context.Context, where string, args []any, pages ...CollectionPage) (error, []GroupSecretSummary) {
	suffix, args, err := store.collectionPageSQL(pages, "secrets.id", []string{"secrets.description"}, args, "secrets.id DESC")
	if err != nil {
		return err, nil
	}
	rows, err := store.QueryContext(ctx, `SELECT `+groupSecretColumns+groupSecretFrom+` WHERE secrets.enabled = TRUE AND `+where+suffix, args...)
	if err != nil {
		return fmt.Errorf("list group secrets: %w", err), nil
	}
	defer rows.Close()
	items := []GroupSecretSummary{}
	for rows.Next() {
		detail, err := scanGroupSecret(rows, false)
		if err != nil {
			return err, nil
		}
		items = append(items, groupSecretSummary(detail))
	}
	return rows.Err(), items
}

func (store *Store) GroupSecretsGet(ctx context.Context, workspace model.WorkspaceRef, group model.GroupRef, principal model.PrincipalRef, pages ...CollectionPage) (error, []GroupSecretSummary) {
	if err, role := store.GroupRoleGet(ctx, workspace.Id, group.Id, principal); err != nil {
		return err, nil
	} else if role == "" {
		return ErrGroupNotFound, nil
	}
	p := store.sessionParticipantPlaceholder
	return store.groupSecretsSearch(ctx, `secrets.workspace = `+p(1)+` AND secrets."group" = `+p(2)+` AND `+groupSecretPrincipalAccessSQL(p(1), p(2), p(3)), []any{workspace.Id, group.Id, principal.Id}, pages...)
}

func (store *Store) GroupSecretGet(ctx context.Context, ref model.GroupSecretRef, principal model.PrincipalRef) (error, *GroupSecretDetail) {
	if !groupSecretRefValid(ref) {
		return ErrGroupSecretNotFound, nil
	}
	p := store.sessionParticipantPlaceholder
	// Metadata-only access for HTTP callers: ciphertext is never loaded here.
	detail, err := scanGroupSecret(store.QueryRowContext(ctx, `SELECT `+groupSecretColumns+groupSecretFrom+`
		WHERE secrets.workspace = `+p(1)+` AND secrets."group" = `+p(2)+` AND secrets.id = `+p(3)+` AND secrets.enabled = TRUE
		AND `+groupSecretPrincipalAccessSQL(p(1), p(2), p(4)), ref.Workspace.Id, ref.Group.Id, ref.Id, principal.Id), false)
	if err == sql.ErrNoRows {
		return ErrGroupSecretNotFound, nil
	}
	if err != nil {
		return err, nil
	}
	return nil, &detail
}

func (store *Store) groupSecretTransaction(ctx context.Context, ref model.GroupSecretRef, actor model.PrincipalRef) (*sql.Tx, error) {
	if !groupSecretRefValid(ref) {
		return nil, ErrGroupSecretInput
	}
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	err = store.lockGroupMembershipUpdate(ctx, tx, ref.Workspace.Id, ref.Group.Id)
	if err == nil {
		err = store.groupMutationAllowed(ctx, tx, ref.Workspace.Id, ref.Group.Id, &actor)
	}
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func (store *Store) GroupSecretCreate(ctx context.Context, secret model.GroupSecret, actor model.PrincipalRef) (error, model.GroupSecret) {
	if err := normalizeGroupSecret(&secret.Description, &secret.Ciphertext); err != nil {
		return err, model.GroupSecret{}
	}
	tx, err := store.groupSecretTransaction(ctx, secret.Ref, actor)
	if err != nil {
		return err, model.GroupSecret{}
	}
	defer tx.Rollback()
	at, err := typed_id.Timestamp(typed_id.GroupSecret, secret.Ref.Id)
	if err != nil {
		return ErrGroupSecretInput, model.GroupSecret{}
	}
	secret.AuthorPrincipal, secret.Enabled, secret.Revision = actor, true, 1
	secret.CreatedAt = at.Format("2006-01-02T15:04:05.000Z")
	secret.UpdatedAt = secret.CreatedAt
	p := keychainPlaceholder(store.kind)
	if _, err := tx.ExecContext(ctx, `INSERT INTO gatehouse_group_secrets (workspace, "group", id, author_principal, description, ciphertext, enabled, revision, created_at, updated_at)
		VALUES (`+p(1)+`, `+p(2)+`, `+p(3)+`, `+p(4)+`, `+p(5)+`, `+p(6)+`, TRUE, 1, `+p(7)+`, `+p(8)+`)`,
		secret.Ref.Workspace.Id, secret.Ref.Group.Id, secret.Ref.Id, actor.Id, secret.Description, secret.Ciphertext, secret.CreatedAt, secret.UpdatedAt); err != nil {
		return err, model.GroupSecret{}
	}
	if err := store.groupSecretActivity(ctx, tx, secret.Ref, "group_secret.create"); err != nil {
		return err, model.GroupSecret{}
	}
	if err := tx.Commit(); err != nil {
		return err, model.GroupSecret{}
	}
	return nil, secret
}

func (store *Store) GroupSecretUpdate(ctx context.Context, ref model.GroupSecretRef, actor model.PrincipalRef, description, ciphertext *string, revision int) (error, *GroupSecretDetail) {
	return store.groupSecretMutate(ctx, ref, actor, description, ciphertext, revision, false)
}

func (store *Store) GroupSecretRemove(ctx context.Context, ref model.GroupSecretRef, actor model.PrincipalRef, revision int) error {
	err, _ := store.groupSecretMutate(ctx, ref, actor, nil, nil, revision, true)
	return err
}

func (store *Store) groupSecretMutate(ctx context.Context, ref model.GroupSecretRef, actor model.PrincipalRef, description, ciphertext *string, revision int, remove bool) (error, *GroupSecretDetail) {
	if revision < 1 || (!remove && description == nil && ciphertext == nil) {
		return ErrGroupSecretInput, nil
	}
	if err := normalizeGroupSecret(description, ciphertext); err != nil {
		return err, nil
	}
	tx, err := store.groupSecretTransaction(ctx, ref, actor)
	if err != nil {
		return err, nil
	}
	defer tx.Rollback()
	p := store.sessionParticipantPlaceholder
	var current int
	err = tx.QueryRowContext(ctx, `SELECT revision FROM gatehouse_group_secrets WHERE workspace = `+p(1)+` AND "group" = `+p(2)+` AND id = `+p(3)+` AND enabled = TRUE`, ref.Workspace.Id, ref.Group.Id, ref.Id).Scan(&current)
	if err == sql.ErrNoRows {
		return ErrGroupSecretNotFound, nil
	}
	if err != nil {
		return err, nil
	}
	if current != revision {
		return ErrRevisionConflict, nil
	}
	if remove {
		empty := ""
		ciphertext = &empty
	}
	_, err = tx.ExecContext(ctx, `UPDATE gatehouse_group_secrets SET description = COALESCE(`+p(1)+`, description), ciphertext = COALESCE(`+p(2)+`, ciphertext),
		enabled = `+p(3)+`, revision = revision + 1, updated_at = `+p(4)+` WHERE workspace = `+p(5)+` AND "group" = `+p(6)+` AND id = `+p(7),
		description, ciphertext, !remove, time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), ref.Workspace.Id, ref.Group.Id, ref.Id)
	if err != nil {
		return err, nil
	}
	event := "group_secret.update"
	if remove {
		event = "group_secret.remove"
	}
	if err := store.groupSecretActivity(ctx, tx, ref, event); err != nil {
		return err, nil
	}
	detail, err := scanGroupSecret(tx.QueryRowContext(ctx, `SELECT `+groupSecretColumns+groupSecretFrom+` WHERE secrets.workspace = `+p(1)+` AND secrets."group" = `+p(2)+` AND secrets.id = `+p(3), ref.Workspace.Id, ref.Group.Id, ref.Id), false)
	if err != nil {
		return err, nil
	}
	detail.Secret.Enabled = !remove
	if err := tx.Commit(); err != nil {
		return err, nil
	}
	return nil, &detail
}

func (store *Store) groupSecretActivity(ctx context.Context, tx *sql.Tx, ref model.GroupSecretRef, event string) error {
	topics, err := groupPrincipalTopics(ctx, tx, keychainPlaceholder(store.kind), ref.Workspace.Id, ref.Group.Id, ref.Id)
	if err != nil {
		return err
	}
	topics = append(topics, ref.Workspace.Id+"/"+ref.Group.Id+"/"+ref.Id)
	err, _ = store.ActivityEventAppend(ctx, tx, model.ActivityEvent{Event: event, ResourceKind: ActivityResourceKindGroupSecret, ResourceGroupSecret: &ref.Id}, topics)
	return err
}

// Both bot operations use this resource-level resolver after round admission.
// Both the acting principal and the executing bot must belong to the owning
// group. These memberships stay live, but chat/project grants and participation
// are deliberately not rechecked midway through an admitted tool batch.
func (store *Store) groupSecretBotAccessSQL(p func(int) string) string {
	return workspacePrincipalAdmissionSQL(p(1), p(2)) + ` AND EXISTS (
			SELECT 1 FROM gatehouse_workspace_agents bot WHERE bot.workspace_id = ` + p(1) + ` AND bot.id = ` + p(3) + ` AND bot.enabled = TRUE)
		AND secrets.workspace = ` + p(1) + ` AND (
			EXISTS (SELECT 1 FROM gatehouse_group_members member WHERE member.workspace_id = secrets.workspace AND member.group_id = secrets."group" AND member.principal_id = ` + p(2) + ` AND member.enabled = TRUE)
			AND EXISTS (SELECT 1 FROM gatehouse_group_agents member WHERE member.workspace_id = secrets.workspace AND member.group_id = secrets."group" AND member.agent_id = ` + p(3) + ` AND member.enabled = TRUE))`
}

func (store *Store) GroupSecretsForBotGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) (error, []GroupSecretSummary) {
	if agent.Workspace != workspace {
		return ErrGroupSecretNotFound, nil
	}
	return store.groupSecretsSearch(ctx, store.groupSecretBotAccessSQL(store.sessionParticipantPlaceholder), []any{workspace.Id, principal.Id, agent.Id})
}

func (store *Store) GroupSecretForBotGet(ctx context.Context, workspace model.WorkspaceRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef, id string) (error, *GroupSecretDetail) {
	if agent.Workspace != workspace || !typed_id.Valid(typed_id.GroupSecret, id) {
		return ErrGroupSecretNotFound, nil
	}
	p := store.sessionParticipantPlaceholder
	detail, err := scanGroupSecret(store.QueryRowContext(ctx, `SELECT `+groupSecretColumns+`, secrets.ciphertext, secrets.enabled`+groupSecretFrom+`
		WHERE secrets.enabled = TRUE AND secrets.id = `+p(4)+` AND `+store.groupSecretBotAccessSQL(p), workspace.Id, principal.Id, agent.Id, id), true)
	if err == sql.ErrNoRows {
		return ErrGroupSecretNotFound, nil
	}
	if err != nil {
		return err, nil
	}
	return nil, &detail
}
