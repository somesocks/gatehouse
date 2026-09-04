package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"gatehouse/model"
	"gatehouse/typed_id"
)

const (
	projectNoteTitleMaxSize       = 256
	projectNoteDescriptionMaxSize = 4 * 1024
	projectNoteBodyMaxSize        = 1024 * 1024
)

type ProjectNoteSummary struct {
	Ref              model.ProjectNoteRef
	AuthorPrincipal  *model.PrincipalRef
	AuthorAgent      *model.WorkspaceAgentRef
	AuthorGateway    *model.GatewayRef
	AuthorName       *string
	AuthorAgentLabel *string
	Title            string
	Description      string
	Sensitive        bool
	CreatedAt        string
	Revision         int
}

type ProjectNoteDetail struct {
	Note             model.ProjectNote
	AuthorName       *string
	AuthorAgentLabel *string
}

type ProjectNoteRevisionSummary struct {
	Ref              model.ProjectNoteRevisionRef
	AuthorPrincipal  *model.PrincipalRef
	AuthorAgent      *model.WorkspaceAgentRef
	AuthorGateway    *model.GatewayRef
	AuthorName       *string
	AuthorAgentLabel *string
	Title            string
	Description      string
	Sensitive        bool
	CreatedAt        string
}

type ProjectNoteRevisionDetail struct {
	Revision         model.ProjectNoteRevision
	AuthorName       *string
	AuthorAgentLabel *string
}

func normalizeProjectNote(title, description, body *string) error {
	if title == nil || description == nil || body == nil {
		return fmt.Errorf("project note fields are required")
	}
	*title = strings.Join(strings.Fields(*title), " ")
	*description = strings.TrimSpace(*description)
	if *title == "" {
		return fmt.Errorf("project note title must not be blank")
	}
	if len(*title) > projectNoteTitleMaxSize || len(*description) > projectNoteDescriptionMaxSize || len(*body) > projectNoteBodyMaxSize {
		return fmt.Errorf("project note title, description, or body exceeds the size limit")
	}
	return nil
}

func (store *Store) ProjectNotesGet(ctx context.Context, project model.ProjectRef, principal model.PrincipalRef) (error, []ProjectNoteSummary) {
	if err, available := store.ProjectGet(ctx, project, principal); err != nil {
		return err, nil
	} else if available == nil {
		return nil, []ProjectNoteSummary{}
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT notes.id, notes.author_principal, principals.name, notes.author_agent, agents.label, notes.author_gateway,
			notes.title, notes.description, revisions.sensitive, notes.created_at, notes.revision
		FROM gatehouse_project_notes AS notes
		JOIN gatehouse_project_note_revisions AS revisions ON revisions.workspace = notes.workspace AND revisions.project = notes.project AND revisions.note = notes.id AND revisions.revision = notes.revision
		LEFT JOIN gatehouse_principals AS principals ON principals.id = notes.author_principal
		LEFT JOIN gatehouse_workspace_agents AS agents ON agents.workspace_id = notes.workspace AND agents.model_id = notes.author_agent
		WHERE notes.workspace = `+placeholder(1)+` AND notes.project = `+placeholder(2)+` AND notes.enabled = TRUE
		ORDER BY notes.created_at DESC, notes.id DESC
	`, project.Workspace.Id, project.Id)
	if err != nil {
		return fmt.Errorf("get project notes: %w", err), nil
	}
	defer rows.Close()
	notes := []ProjectNoteSummary{}
	for rows.Next() {
		var note ProjectNoteSummary
		var authorPrincipal, authorName, authorAgent, authorAgentLabel, authorGateway sql.NullString
		note.Ref.Project = project
		if err := rows.Scan(&note.Ref.Id, &authorPrincipal, &authorName, &authorAgent, &authorAgentLabel, &authorGateway, &note.Title, &note.Description, &note.Sensitive, &note.CreatedAt, &note.Revision); err != nil {
			return fmt.Errorf("scan project note: %w", err), nil
		}
		author := noteAuthorFromValues(project.Workspace, authorPrincipal, authorAgent, authorGateway)
		note.AuthorPrincipal, note.AuthorAgent, note.AuthorGateway = author.Principal, author.Agent, author.Gateway
		if authorName.Valid {
			note.AuthorName = &authorName.String
		}
		if authorAgentLabel.Valid {
			note.AuthorAgentLabel = &authorAgentLabel.String
		}
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project notes: %w", err), nil
	}
	return nil, notes
}

func (store *Store) ProjectNoteGet(ctx context.Context, note model.ProjectNoteRef, principal model.PrincipalRef) (error, *ProjectNoteDetail) {
	if !typed_id.Valid(typed_id.ProjectNote, note.Id) {
		return fmt.Errorf("get project note: ID is invalid"), nil
	}
	if err, project := store.ProjectGet(ctx, note.Project, principal); err != nil {
		return err, nil
	} else if project == nil {
		return nil, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT notes.author_principal, principals.name, notes.author_agent, agents.label, notes.author_gateway,
			notes.title, notes.description, notes.body, revisions.sensitive, notes.enabled, notes.created_at, notes.revision
		FROM gatehouse_project_notes AS notes
		JOIN gatehouse_project_note_revisions AS revisions ON revisions.workspace = notes.workspace AND revisions.project = notes.project AND revisions.note = notes.id AND revisions.revision = notes.revision
		LEFT JOIN gatehouse_principals AS principals ON principals.id = notes.author_principal
		LEFT JOIN gatehouse_workspace_agents AS agents ON agents.workspace_id = notes.workspace AND agents.model_id = notes.author_agent
		WHERE notes.workspace = `+placeholder(1)+` AND notes.project = `+placeholder(2)+` AND notes.id = `+placeholder(3)+` AND notes.enabled = TRUE
	`, note.Project.Workspace.Id, note.Project.Id, note.Id)
	detail := &ProjectNoteDetail{Note: model.ProjectNote{Ref: note}}
	var authorPrincipal, authorName, authorAgent, authorAgentLabel, authorGateway sql.NullString
	if err := row.Scan(&authorPrincipal, &authorName, &authorAgent, &authorAgentLabel, &authorGateway, &detail.Note.Title, &detail.Note.Description, &detail.Note.Body, &detail.Note.Sensitive, &detail.Note.Enabled, &detail.Note.CreatedAt, &detail.Note.Revision); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get project note: %w", err), nil
	}
	if authorName.Valid {
		detail.AuthorName = &authorName.String
	}
	if authorAgentLabel.Valid {
		detail.AuthorAgentLabel = &authorAgentLabel.String
	}
	author := noteAuthorFromValues(note.Project.Workspace, authorPrincipal, authorAgent, authorGateway)
	detail.Note.AuthorPrincipal, detail.Note.AuthorAgent, detail.Note.AuthorGateway = author.Principal, author.Agent, author.Gateway
	return nil, detail
}

func (store *Store) ProjectNoteCreate(ctx context.Context, note model.ProjectNote, principal model.PrincipalRef) (error, model.ProjectNote) {
	if !typed_id.Valid(typed_id.ProjectNote, note.Ref.Id) || strings.TrimSpace(principal.Id) == "" {
		return fmt.Errorf("create project note: IDs are invalid"), model.ProjectNote{}
	}
	if err := normalizeProjectNote(&note.Title, &note.Description, &note.Body); err != nil {
		return fmt.Errorf("create project note: %w", err), model.ProjectNote{}
	}
	if err, project := store.ProjectGet(ctx, note.Ref.Project, principal); err != nil {
		return err, model.ProjectNote{}
	} else if project == nil {
		return fmt.Errorf("create project note: project is unavailable"), model.ProjectNote{}
	}
	createdAt, err := typed_id.Timestamp(typed_id.ProjectNote, note.Ref.Id)
	if err != nil {
		return fmt.Errorf("create project note: note ID is invalid"), model.ProjectNote{}
	}
	author := NoteAuthor{Principal: note.AuthorPrincipal, Agent: note.AuthorAgent, Gateway: note.AuthorGateway}
	if author.Principal == nil && author.Agent == nil && author.Gateway == nil {
		author.Principal = &principal
	}
	if err := noteAuthorValid(author, note.Ref.Project.Workspace); err != nil {
		return fmt.Errorf("create project note: %w", err), model.ProjectNote{}
	}
	note.AuthorPrincipal, note.AuthorAgent, note.AuthorGateway = author.Principal, author.Agent, author.Gateway
	note.Enabled = true
	note.Revision = 1
	note.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project note creation: %w", err), model.ProjectNote{}
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	principalID, agentID, gatewayID := noteAuthorValues(author)
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_project_notes (workspace, project, id, author_principal, author_agent, author_gateway, title, description, body, enabled, created_at, revision)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`, TRUE, `+placeholder(10)+`, `+placeholder(11)+`)
	`, note.Ref.Project.Workspace.Id, note.Ref.Project.Id, note.Ref.Id, principalID, agentID, gatewayID, note.Title, note.Description, note.Body, note.CreatedAt, note.Revision); err != nil {
		return fmt.Errorf("insert project note: %w", err), model.ProjectNote{}
	}
	if err := store.projectNoteRevisionInsert(ctx, transaction, note.Ref, note.Revision, author, note.Title, note.Description, note.Body, note.Sensitive, note.CreatedAt); err != nil {
		return err, model.ProjectNote{}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{},
		Event:        "project_note.create",
		ResourceKind: ActivityResourceKindProjectNote,
		ResourceProjectNote: &note.Ref.Id,
	}, []string{ActivityTopicProjectNote(note.Ref)}); err != nil {
		return fmt.Errorf("append project note creation activity: %w", err), model.ProjectNote{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project note creation: %w", err), model.ProjectNote{}
	}
	return nil, note
}

func (store *Store) ProjectNoteDetailsSetAs(ctx context.Context, note model.ProjectNoteRef, principal model.PrincipalRef, author NoteAuthor, sensitive bool, title, description, body *string) (error, *ProjectNoteDetail) {
	if err := normalizeProjectNote(title, description, body); err != nil {
		return fmt.Errorf("update project note: %w", err), nil
	}
	if err := noteAuthorValid(author, note.Project.Workspace); err != nil {
		return fmt.Errorf("update project note: %w", err), nil
	}
	err, detail := store.ProjectNoteGet(ctx, note, principal)
	if err != nil || detail == nil {
		return err, detail
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project note update: %w", err), nil
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	principalID, agentID, gatewayID := noteAuthorValues(author)
	row := transaction.QueryRowContext(ctx, `
		UPDATE gatehouse_project_notes
		SET author_principal = `+placeholder(1)+`, author_agent = `+placeholder(2)+`, author_gateway = `+placeholder(3)+`,
			title = `+placeholder(4)+`, description = `+placeholder(5)+`, body = `+placeholder(6)+`, revision = revision + 1
		WHERE workspace = `+placeholder(7)+` AND project = `+placeholder(8)+` AND id = `+placeholder(9)+`
			AND enabled = TRUE
		RETURNING revision
	`, principalID, agentID, gatewayID, *title, *description, *body, note.Project.Workspace.Id, note.Project.Id, note.Id)
	var nextRevision int
	if err := row.Scan(&nextRevision); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("update project note: %w", err), nil
	}
	createdAt := noteRevisionCreatedAt()
	if err := store.projectNoteRevisionInsert(ctx, transaction, note, nextRevision, author, *title, *description, *body, sensitive, createdAt); err != nil {
		return err, nil
	}
	detail.Note.Title, detail.Note.Description, detail.Note.Body, detail.Note.Sensitive, detail.Note.Revision = *title, *description, *body, sensitive, nextRevision
	detail.Note.AuthorPrincipal, detail.Note.AuthorAgent, detail.Note.AuthorGateway = author.Principal, author.Agent, author.Gateway
	detail.AuthorName, detail.AuthorAgentLabel = nil, nil
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{},
		Event:        "project_note.update",
		ResourceKind: ActivityResourceKindProjectNote,
		ResourceProjectNote: &note.Id,
	}, []string{ActivityTopicProjectNote(note)}); err != nil {
		return fmt.Errorf("append project note update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project note update: %w", err), nil
	}
	return nil, detail
}

func (store *Store) projectNoteRevisionInsert(ctx context.Context, transaction *sql.Tx, note model.ProjectNoteRef, revision int, author NoteAuthor, title, description, body string, sensitive bool, createdAt string) error {
	if err := noteAuthorValid(author, note.Project.Workspace); err != nil {
		return fmt.Errorf("insert project note revision: %w", err)
	}
	principalID, agentID, gatewayID := noteAuthorValues(author)
	placeholder := keychainPlaceholder(store.kind)
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_project_note_revisions (
			workspace, project, note, revision, author_principal, author_agent, author_gateway,
			title, description, body, sensitive, created_at
		) VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`, `+placeholder(10)+`, `+placeholder(11)+`, `+placeholder(12)+`)
	`, note.Project.Workspace.Id, note.Project.Id, note.Id, revision, principalID, agentID, gatewayID, title, description, body, sensitive, createdAt); err != nil {
		return fmt.Errorf("insert project note revision: %w", err)
	}
	return nil
}

func (store *Store) ProjectNoteRemove(ctx context.Context, note model.ProjectNoteRef, principal model.PrincipalRef) (error, bool) {
	if !typed_id.Valid(typed_id.ProjectNote, note.Id) {
		return fmt.Errorf("remove project note: ID is invalid"), false
	}
	if err, project := store.ProjectGet(ctx, note.Project, principal); err != nil {
		return err, false
	} else if project == nil {
		return nil, false
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project note removal: %w", err), false
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	result, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_project_notes SET enabled = FALSE
		WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND id = `+placeholder(3)+` AND enabled = TRUE
	`, note.Project.Workspace.Id, note.Project.Id, note.Id)
	if err != nil {
		return fmt.Errorf("remove project note: %w", err), false
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove project note: %w", err), false
	}
	if changed != 1 {
		return nil, false
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{},
		Event:        "project_note.remove",
		ResourceKind: ActivityResourceKindProjectNote,
		ResourceProjectNote: &note.Id,
	}, []string{ActivityTopicProjectNote(note)}); err != nil {
		return fmt.Errorf("append project note removal activity: %w", err), false
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project note removal: %w", err), false
	}
	return nil, true
}

func (store *Store) ProjectNoteRevisionsGet(ctx context.Context, note model.ProjectNoteRef, principal model.PrincipalRef) (error, []ProjectNoteRevisionSummary) {
	if err, current := store.ProjectNoteGet(ctx, note, principal); err != nil {
		return err, nil
	} else if current == nil {
		return nil, []ProjectNoteRevisionSummary{}
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT revisions.revision, revisions.author_principal, principals.name, revisions.author_agent, agents.label, revisions.author_gateway,
			revisions.title, revisions.description, revisions.sensitive, revisions.created_at
		FROM gatehouse_project_note_revisions AS revisions
		LEFT JOIN gatehouse_principals AS principals ON principals.id = revisions.author_principal
		LEFT JOIN gatehouse_workspace_agents AS agents ON agents.workspace_id = revisions.workspace AND agents.model_id = revisions.author_agent
		WHERE revisions.workspace = `+placeholder(1)+` AND revisions.project = `+placeholder(2)+` AND revisions.note = `+placeholder(3)+`
		ORDER BY revisions.revision DESC
	`, note.Project.Workspace.Id, note.Project.Id, note.Id)
	if err != nil {
		return fmt.Errorf("get project note revisions: %w", err), nil
	}
	defer rows.Close()
	revisions := []ProjectNoteRevisionSummary{}
	for rows.Next() {
		var revision ProjectNoteRevisionSummary
		var authorPrincipal, authorName, authorAgent, authorAgentLabel, authorGateway sql.NullString
		revision.Ref.Note = note
		if err := rows.Scan(&revision.Ref.Revision, &authorPrincipal, &authorName, &authorAgent, &authorAgentLabel, &authorGateway, &revision.Title, &revision.Description, &revision.Sensitive, &revision.CreatedAt); err != nil {
			return fmt.Errorf("scan project note revision: %w", err), nil
		}
		author := noteAuthorFromValues(note.Project.Workspace, authorPrincipal, authorAgent, authorGateway)
		revision.AuthorPrincipal, revision.AuthorAgent, revision.AuthorGateway = author.Principal, author.Agent, author.Gateway
		if authorName.Valid {
			revision.AuthorName = &authorName.String
		}
		if authorAgentLabel.Valid {
			revision.AuthorAgentLabel = &authorAgentLabel.String
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project note revisions: %w", err), nil
	}
	return nil, revisions
}

func (store *Store) ProjectNoteRevisionGet(ctx context.Context, revision model.ProjectNoteRevisionRef, principal model.PrincipalRef) (error, *ProjectNoteRevisionDetail) {
	if revision.Revision < 1 {
		return fmt.Errorf("get project note revision: revision is invalid"), nil
	}
	if err, current := store.ProjectNoteGet(ctx, revision.Note, principal); err != nil {
		return err, nil
	} else if current == nil {
		return nil, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT revisions.author_principal, principals.name, revisions.author_agent, agents.label, revisions.author_gateway,
			revisions.title, revisions.description, revisions.body, revisions.sensitive, revisions.created_at
		FROM gatehouse_project_note_revisions AS revisions
		LEFT JOIN gatehouse_principals AS principals ON principals.id = revisions.author_principal
		LEFT JOIN gatehouse_workspace_agents AS agents ON agents.workspace_id = revisions.workspace AND agents.model_id = revisions.author_agent
		WHERE revisions.workspace = `+placeholder(1)+` AND revisions.project = `+placeholder(2)+` AND revisions.note = `+placeholder(3)+` AND revisions.revision = `+placeholder(4)+`
	`, revision.Note.Project.Workspace.Id, revision.Note.Project.Id, revision.Note.Id, revision.Revision)
	detail := &ProjectNoteRevisionDetail{Revision: model.ProjectNoteRevision{Ref: revision}}
	var authorPrincipal, authorName, authorAgent, authorAgentLabel, authorGateway sql.NullString
	if err := row.Scan(&authorPrincipal, &authorName, &authorAgent, &authorAgentLabel, &authorGateway, &detail.Revision.Title, &detail.Revision.Description, &detail.Revision.Body, &detail.Revision.Sensitive, &detail.Revision.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get project note revision: %w", err), nil
	}
	author := noteAuthorFromValues(revision.Note.Project.Workspace, authorPrincipal, authorAgent, authorGateway)
	detail.Revision.AuthorPrincipal, detail.Revision.AuthorAgent, detail.Revision.AuthorGateway = author.Principal, author.Agent, author.Gateway
	if authorName.Valid {
		detail.AuthorName = &authorName.String
	}
	if authorAgentLabel.Valid {
		detail.AuthorAgentLabel = &authorAgentLabel.String
	}
	return nil, detail
}
