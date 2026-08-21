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
	Ref             model.SessionNoteRef
	AuthorPrincipal model.PrincipalRef
	AuthorName      *string
	Title           string
	Description     string
	Sensitive       bool
	CreatedAt       string
}

type SessionNoteDetail struct {
	Note       model.SessionNote
	AuthorName *string
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
		SELECT notes.id, notes.author_principal, principals.name, notes.title, notes.description, notes.sensitive, notes.created_at
		FROM gatehouse_session_notes AS notes
		JOIN gatehouse_principals AS principals ON principals.id = notes.author_principal
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
		var authorName sql.NullString
		note.Ref.Session = session
		if err := rows.Scan(&note.Ref.Id, &note.AuthorPrincipal.Id, &authorName, &note.Title, &note.Description, &note.Sensitive, &note.CreatedAt); err != nil {
			return fmt.Errorf("scan session note: %w", err), nil
		}
		if authorName.Valid {
			note.AuthorName = &authorName.String
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
		SELECT notes.author_principal, principals.name, notes.title, notes.description, notes.body, notes.sensitive, notes.enabled, notes.created_at
		FROM gatehouse_session_notes AS notes
		JOIN gatehouse_principals AS principals ON principals.id = notes.author_principal
		WHERE notes.workspace = `+placeholder(1)+` AND notes.session = `+placeholder(2)+` AND notes.id = `+placeholder(3)+` AND notes.enabled = TRUE
	`, note.Session.Workspace.Id, note.Session.Id, note.Id)
	detail := &SessionNoteDetail{Note: model.SessionNote{Ref: note}}
	var authorName sql.NullString
	if err := row.Scan(&detail.Note.AuthorPrincipal.Id, &authorName, &detail.Note.Title, &detail.Note.Description, &detail.Note.Body, &detail.Note.Sensitive, &detail.Note.Enabled, &detail.Note.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get session note: %w", err), nil
	}
	if authorName.Valid {
		detail.AuthorName = &authorName.String
	}
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
	note.AuthorPrincipal = principal
	note.Enabled = true
	note.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session note creation: %w", err), model.SessionNote{}
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_session_notes (workspace, session, id, author_principal, title, description, body, sensitive, enabled, created_at)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, TRUE, `+placeholder(9)+`)
	`, note.Ref.Session.Workspace.Id, note.Ref.Session.Id, note.Ref.Id, note.AuthorPrincipal.Id, note.Title, note.Description, note.Body, note.Sensitive, note.CreatedAt); err != nil {
		return fmt.Errorf("insert session note: %w", err), model.SessionNote{}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{Workspace: note.Ref.Session.Workspace},
		Event:        "session_note.create",
		ResourceKind: ActivityResourceKindSession,
		Session:      &note.Ref.Session,
	}, []string{ActivityTopicSession(note.Ref.Session)}); err != nil {
		return fmt.Errorf("append session note creation activity: %w", err), model.SessionNote{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session note creation: %w", err), model.SessionNote{}
	}
	return nil, note
}

func (store *Store) SessionNoteDetailsSet(ctx context.Context, note model.SessionNoteRef, principal model.PrincipalRef, title, description, body *string) (error, *SessionNoteDetail) {
	if err := normalizeSessionNote(title, description, body); err != nil {
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
	result, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_session_notes SET title = `+placeholder(1)+`, description = `+placeholder(2)+`, body = `+placeholder(3)+`
		WHERE workspace = `+placeholder(4)+` AND session = `+placeholder(5)+` AND id = `+placeholder(6)+` AND enabled = TRUE
	`, *title, *description, *body, note.Session.Workspace.Id, note.Session.Id, note.Id)
	if err != nil {
		return fmt.Errorf("update session note: %w", err), nil
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update session note: %w", err), nil
	}
	if changed != 1 {
		return nil, nil
	}
	detail.Note.Title, detail.Note.Description, detail.Note.Body = *title, *description, *body
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{Workspace: note.Session.Workspace},
		Event:        "session_note.update",
		ResourceKind: ActivityResourceKindSession,
		Session:      &note.Session,
	}, []string{ActivityTopicSession(note.Session)}); err != nil {
		return fmt.Errorf("append session note update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session note update: %w", err), nil
	}
	return nil, detail
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
		Ref:          model.ActivityEventRef{Workspace: note.Session.Workspace},
		Event:        "session_note.remove",
		ResourceKind: ActivityResourceKindSession,
		Session:      &note.Session,
	}, []string{ActivityTopicSession(note.Session)}); err != nil {
		return fmt.Errorf("append session note removal activity: %w", err), false
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session note removal: %w", err), false
	}
	return nil, true
}
