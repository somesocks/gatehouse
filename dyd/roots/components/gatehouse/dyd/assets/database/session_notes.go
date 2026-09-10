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
	sessionNoteTitleMaxSize       = 256
	sessionNoteDescriptionMaxSize = 4 * 1024
	sessionNoteBodyMaxSize        = 1024 * 1024
)

type SessionNoteSummary struct {
	Ref              model.SessionNoteRef
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

type SessionNoteDetail struct {
	Note             model.SessionNote
	AuthorName       *string
	AuthorAgentLabel *string
}

type SessionNoteRevisionSummary struct {
	Ref              model.SessionNoteRevisionRef
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

type SessionNoteRevisionDetail struct {
	Revision         model.SessionNoteRevision
	AuthorName       *string
	AuthorAgentLabel *string
}

func normalizeSessionNote(title, description, body *string) error {
	if title == nil || description == nil || body == nil {
		return fmt.Errorf("session note fields are required")
	}
	*title = strings.Join(strings.Fields(*title), " ")
	*description = strings.TrimSpace(*description)
	if *title == "" {
		return fmt.Errorf("session note title must not be blank")
	}
	if len(*title) > sessionNoteTitleMaxSize || len(*description) > sessionNoteDescriptionMaxSize || len(*body) > sessionNoteBodyMaxSize {
		return fmt.Errorf("session note title, description, or body exceeds the size limit")
	}
	return nil
}

func (store *Store) SessionNotesGet(ctx context.Context, session model.SessionRef, principal model.PrincipalRef) (error, []SessionNoteSummary) {
	if err, available := store.SessionGet(ctx, session, principal); err != nil {
		return err, nil
	} else if available == nil {
		return nil, []SessionNoteSummary{}
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT notes.id, notes.author_principal, principals.name, notes.author_agent, agents.label, notes.author_gateway,
			notes.title, notes.description, revisions.sensitive, notes.created_at, notes.revision
		FROM gatehouse_session_notes AS notes
		JOIN gatehouse_session_note_revisions AS revisions ON revisions.workspace = notes.workspace AND revisions.session = notes.session AND revisions.note = notes.id AND revisions.revision = notes.revision
		LEFT JOIN gatehouse_principals AS principals ON principals.id = notes.author_principal
		LEFT JOIN gatehouse_workspace_agents AS agents ON agents.workspace_id = notes.workspace AND agents.id = notes.author_agent
		WHERE notes.workspace = `+placeholder(1)+` AND notes.session = `+placeholder(2)+` AND notes.enabled = TRUE
		ORDER BY notes.created_at DESC, notes.id DESC
	`, session.Workspace.Id, session.Id)
	if err != nil {
		return fmt.Errorf("get session notes: %w", err), nil
	}
	defer rows.Close()
	notes := []SessionNoteSummary{}
	for rows.Next() {
		var note SessionNoteSummary
		var authorPrincipal, authorName, authorAgent, authorAgentLabel, authorGateway sql.NullString
		note.Ref.Session = session
		if err := rows.Scan(&note.Ref.Id, &authorPrincipal, &authorName, &authorAgent, &authorAgentLabel, &authorGateway, &note.Title, &note.Description, &note.Sensitive, &note.CreatedAt, &note.Revision); err != nil {
			return fmt.Errorf("scan session note: %w", err), nil
		}
		author := noteAuthorFromValues(session.Workspace, authorPrincipal, authorAgent, authorGateway)
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
		return fmt.Errorf("iterate session notes: %w", err), nil
	}
	return nil, notes
}

func (store *Store) SessionNoteGet(ctx context.Context, note model.SessionNoteRef, principal model.PrincipalRef) (error, *SessionNoteDetail) {
	if !typed_id.Valid(typed_id.SessionNote, note.Id) {
		return fmt.Errorf("get session note: ID is invalid"), nil
	}
	if err, session := store.SessionGet(ctx, note.Session, principal); err != nil {
		return err, nil
	} else if session == nil {
		return nil, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT notes.author_principal, principals.name, notes.author_agent, agents.label, notes.author_gateway,
			notes.title, notes.description, notes.body, revisions.sensitive, notes.enabled, notes.created_at, notes.revision
		FROM gatehouse_session_notes AS notes
		JOIN gatehouse_session_note_revisions AS revisions ON revisions.workspace = notes.workspace AND revisions.session = notes.session AND revisions.note = notes.id AND revisions.revision = notes.revision
		LEFT JOIN gatehouse_principals AS principals ON principals.id = notes.author_principal
		LEFT JOIN gatehouse_workspace_agents AS agents ON agents.workspace_id = notes.workspace AND agents.id = notes.author_agent
		WHERE notes.workspace = `+placeholder(1)+` AND notes.session = `+placeholder(2)+` AND notes.id = `+placeholder(3)+` AND notes.enabled = TRUE
	`, note.Session.Workspace.Id, note.Session.Id, note.Id)
	detail := &SessionNoteDetail{Note: model.SessionNote{Ref: note}}
	var authorPrincipal, authorName, authorAgent, authorAgentLabel, authorGateway sql.NullString
	if err := row.Scan(&authorPrincipal, &authorName, &authorAgent, &authorAgentLabel, &authorGateway, &detail.Note.Title, &detail.Note.Description, &detail.Note.Body, &detail.Note.Sensitive, &detail.Note.Enabled, &detail.Note.CreatedAt, &detail.Note.Revision); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get session note: %w", err), nil
	}
	if authorName.Valid {
		detail.AuthorName = &authorName.String
	}
	if authorAgentLabel.Valid {
		detail.AuthorAgentLabel = &authorAgentLabel.String
	}
	author := noteAuthorFromValues(note.Session.Workspace, authorPrincipal, authorAgent, authorGateway)
	detail.Note.AuthorPrincipal, detail.Note.AuthorAgent, detail.Note.AuthorGateway = author.Principal, author.Agent, author.Gateway
	return nil, detail
}

func (store *Store) SessionNoteCreate(ctx context.Context, note model.SessionNote, principal model.PrincipalRef) (error, model.SessionNote) {
	if !typed_id.Valid(typed_id.SessionNote, note.Ref.Id) || strings.TrimSpace(principal.Id) == "" {
		return fmt.Errorf("create session note: IDs are invalid"), model.SessionNote{}
	}
	if err := normalizeSessionNote(&note.Title, &note.Description, &note.Body); err != nil {
		return fmt.Errorf("create session note: %w", err), model.SessionNote{}
	}
	if err, session := store.SessionGet(ctx, note.Ref.Session, principal); err != nil {
		return err, model.SessionNote{}
	} else if session == nil {
		return fmt.Errorf("create session note: session is unavailable"), model.SessionNote{}
	}
	createdAt, err := typed_id.Timestamp(typed_id.SessionNote, note.Ref.Id)
	if err != nil {
		return fmt.Errorf("create session note: note ID is invalid"), model.SessionNote{}
	}
	author := NoteAuthor{Principal: note.AuthorPrincipal, Agent: note.AuthorAgent, Gateway: note.AuthorGateway}
	if author.Principal == nil && author.Agent == nil && author.Gateway == nil {
		author.Principal = &principal
	}
	if err := noteAuthorValid(author, note.Ref.Session.Workspace); err != nil {
		return fmt.Errorf("create session note: %w", err), model.SessionNote{}
	}
	note.AuthorPrincipal, note.AuthorAgent, note.AuthorGateway = author.Principal, author.Agent, author.Gateway
	note.Enabled = true
	note.Revision = 1
	note.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session note creation: %w", err), model.SessionNote{}
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	principalID, agentID, gatewayID := noteAuthorValues(author)
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_session_notes (workspace, session, id, author_principal, author_agent, author_gateway, title, description, body, enabled, created_at, revision)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`, TRUE, `+placeholder(10)+`, `+placeholder(11)+`)
	`, note.Ref.Session.Workspace.Id, note.Ref.Session.Id, note.Ref.Id, principalID, agentID, gatewayID, note.Title, note.Description, note.Body, note.CreatedAt, note.Revision); err != nil {
		return fmt.Errorf("insert session note: %w", err), model.SessionNote{}
	}
	if err := store.sessionNoteRevisionInsert(ctx, transaction, note.Ref, note.Revision, author, note.Title, note.Description, note.Body, note.Sensitive, note.CreatedAt); err != nil {
		return err, model.SessionNote{}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{},
		Event:        "session_note.create",
		ResourceKind: ActivityResourceKindSessionNote,
		ResourceSessionNote: &note.Ref.Id,
	}, []string{ActivityTopicSessionNote(note.Ref)}); err != nil {
		return fmt.Errorf("append session note creation activity: %w", err), model.SessionNote{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session note creation: %w", err), model.SessionNote{}
	}
	return nil, note
}

func (store *Store) SessionNoteDetailsSetAs(ctx context.Context, note model.SessionNoteRef, principal model.PrincipalRef, author NoteAuthor, sensitive bool, title, description, body *string) (error, *SessionNoteDetail) {
	if err := normalizeSessionNote(title, description, body); err != nil {
		return fmt.Errorf("update session note: %w", err), nil
	}
	if err := noteAuthorValid(author, note.Session.Workspace); err != nil {
		return fmt.Errorf("update session note: %w", err), nil
	}
	err, detail := store.SessionNoteGet(ctx, note, principal)
	if err != nil || detail == nil {
		return err, detail
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session note update: %w", err), nil
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	principalID, agentID, gatewayID := noteAuthorValues(author)
	row := transaction.QueryRowContext(ctx, `
		UPDATE gatehouse_session_notes
		SET author_principal = `+placeholder(1)+`, author_agent = `+placeholder(2)+`, author_gateway = `+placeholder(3)+`,
			title = `+placeholder(4)+`, description = `+placeholder(5)+`, body = `+placeholder(6)+`, revision = revision + 1
		WHERE workspace = `+placeholder(7)+` AND session = `+placeholder(8)+` AND id = `+placeholder(9)+` AND enabled = TRUE
		RETURNING revision
	`, principalID, agentID, gatewayID, *title, *description, *body, note.Session.Workspace.Id, note.Session.Id, note.Id)
	var nextRevision int
	if err := row.Scan(&nextRevision); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("update session note: %w", err), nil
	}
	createdAt := noteRevisionCreatedAt()
	if err := store.sessionNoteRevisionInsert(ctx, transaction, note, nextRevision, author, *title, *description, *body, sensitive, createdAt); err != nil {
		return err, nil
	}
	detail.Note.Title, detail.Note.Description, detail.Note.Body, detail.Note.Sensitive, detail.Note.Revision = *title, *description, *body, sensitive, nextRevision
	detail.Note.AuthorPrincipal, detail.Note.AuthorAgent, detail.Note.AuthorGateway = author.Principal, author.Agent, author.Gateway
	detail.AuthorName, detail.AuthorAgentLabel = nil, nil
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{},
		Event:        "session_note.update",
		ResourceKind: ActivityResourceKindSessionNote,
		ResourceSessionNote: &note.Id,
	}, []string{ActivityTopicSessionNote(note)}); err != nil {
		return fmt.Errorf("append session note update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session note update: %w", err), nil
	}
	return nil, detail
}

func (store *Store) sessionNoteRevisionInsert(ctx context.Context, transaction *sql.Tx, note model.SessionNoteRef, revision int, author NoteAuthor, title, description, body string, sensitive bool, createdAt string) error {
	if err := noteAuthorValid(author, note.Session.Workspace); err != nil {
		return fmt.Errorf("insert session note revision: %w", err)
	}
	principalID, agentID, gatewayID := noteAuthorValues(author)
	placeholder := keychainPlaceholder(store.kind)
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_session_note_revisions (
			workspace, session, note, revision, author_principal, author_agent, author_gateway,
			title, description, body, sensitive, created_at
		) VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`, `+placeholder(10)+`, `+placeholder(11)+`, `+placeholder(12)+`)
	`, note.Session.Workspace.Id, note.Session.Id, note.Id, revision, principalID, agentID, gatewayID, title, description, body, sensitive, createdAt); err != nil {
		return fmt.Errorf("insert session note revision: %w", err)
	}
	return nil
}

func (store *Store) SessionNoteRemove(ctx context.Context, note model.SessionNoteRef, principal model.PrincipalRef) (error, bool) {
	if !typed_id.Valid(typed_id.SessionNote, note.Id) {
		return fmt.Errorf("remove session note: ID is invalid"), false
	}
	if err, session := store.SessionGet(ctx, note.Session, principal); err != nil {
		return err, false
	} else if session == nil {
		return nil, false
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session note removal: %w", err), false
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	result, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_session_notes SET enabled = FALSE
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND id = `+placeholder(3)+` AND enabled = TRUE
	`, note.Session.Workspace.Id, note.Session.Id, note.Id)
	if err != nil {
		return fmt.Errorf("remove session note: %w", err), false
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove session note: %w", err), false
	}
	if changed != 1 {
		return nil, false
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{},
		Event:        "session_note.remove",
		ResourceKind: ActivityResourceKindSessionNote,
		ResourceSessionNote: &note.Id,
	}, []string{ActivityTopicSessionNote(note)}); err != nil {
		return fmt.Errorf("append session note removal activity: %w", err), false
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session note removal: %w", err), false
	}
	return nil, true
}

func (store *Store) SessionNoteRevisionsGet(ctx context.Context, note model.SessionNoteRef, principal model.PrincipalRef) (error, []SessionNoteRevisionSummary) {
	if err, current := store.SessionNoteGet(ctx, note, principal); err != nil {
		return err, nil
	} else if current == nil {
		return nil, []SessionNoteRevisionSummary{}
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT revisions.revision, revisions.author_principal, principals.name, revisions.author_agent, agents.label, revisions.author_gateway,
			revisions.title, revisions.description, revisions.sensitive, revisions.created_at
		FROM gatehouse_session_note_revisions AS revisions
		LEFT JOIN gatehouse_principals AS principals ON principals.id = revisions.author_principal
		LEFT JOIN gatehouse_workspace_agents AS agents ON agents.workspace_id = revisions.workspace AND agents.id = revisions.author_agent
		WHERE revisions.workspace = `+placeholder(1)+` AND revisions.session = `+placeholder(2)+` AND revisions.note = `+placeholder(3)+`
		ORDER BY revisions.revision DESC
	`, note.Session.Workspace.Id, note.Session.Id, note.Id)
	if err != nil {
		return fmt.Errorf("get session note revisions: %w", err), nil
	}
	defer rows.Close()
	revisions := []SessionNoteRevisionSummary{}
	for rows.Next() {
		var revision SessionNoteRevisionSummary
		var authorPrincipal, authorName, authorAgent, authorAgentLabel, authorGateway sql.NullString
		revision.Ref.Note = note
		if err := rows.Scan(&revision.Ref.Revision, &authorPrincipal, &authorName, &authorAgent, &authorAgentLabel, &authorGateway, &revision.Title, &revision.Description, &revision.Sensitive, &revision.CreatedAt); err != nil {
			return fmt.Errorf("scan session note revision: %w", err), nil
		}
		author := noteAuthorFromValues(note.Session.Workspace, authorPrincipal, authorAgent, authorGateway)
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
		return fmt.Errorf("iterate session note revisions: %w", err), nil
	}
	return nil, revisions
}

func (store *Store) SessionNoteRevisionGet(ctx context.Context, revision model.SessionNoteRevisionRef, principal model.PrincipalRef) (error, *SessionNoteRevisionDetail) {
	if revision.Revision < 1 {
		return fmt.Errorf("get session note revision: revision is invalid"), nil
	}
	if err, current := store.SessionNoteGet(ctx, revision.Note, principal); err != nil {
		return err, nil
	} else if current == nil {
		return nil, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT revisions.author_principal, principals.name, revisions.author_agent, agents.label, revisions.author_gateway,
			revisions.title, revisions.description, revisions.body, revisions.sensitive, revisions.created_at
		FROM gatehouse_session_note_revisions AS revisions
		LEFT JOIN gatehouse_principals AS principals ON principals.id = revisions.author_principal
		LEFT JOIN gatehouse_workspace_agents AS agents ON agents.workspace_id = revisions.workspace AND agents.id = revisions.author_agent
		WHERE revisions.workspace = `+placeholder(1)+` AND revisions.session = `+placeholder(2)+` AND revisions.note = `+placeholder(3)+` AND revisions.revision = `+placeholder(4)+`
	`, revision.Note.Session.Workspace.Id, revision.Note.Session.Id, revision.Note.Id, revision.Revision)
	detail := &SessionNoteRevisionDetail{Revision: model.SessionNoteRevision{Ref: revision}}
	var authorPrincipal, authorName, authorAgent, authorAgentLabel, authorGateway sql.NullString
	if err := row.Scan(&authorPrincipal, &authorName, &authorAgent, &authorAgentLabel, &authorGateway, &detail.Revision.Title, &detail.Revision.Description, &detail.Revision.Body, &detail.Revision.Sensitive, &detail.Revision.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get session note revision: %w", err), nil
	}
	author := noteAuthorFromValues(revision.Note.Session.Workspace, authorPrincipal, authorAgent, authorGateway)
	detail.Revision.AuthorPrincipal, detail.Revision.AuthorAgent, detail.Revision.AuthorGateway = author.Principal, author.Agent, author.Gateway
	if authorName.Valid {
		detail.AuthorName = &authorName.String
	}
	if authorAgentLabel.Valid {
		detail.AuthorAgentLabel = &authorAgentLabel.String
	}
	return nil, detail
}
