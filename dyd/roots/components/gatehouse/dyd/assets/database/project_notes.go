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
	Ref             model.ProjectNoteRef
	AuthorPrincipal model.PrincipalRef
	AuthorName      *string
	Title           string
	Description     string
	CreatedAt       string
}

type ProjectNoteDetail struct {
	Note       model.ProjectNote
	AuthorName *string
}

func normalizeProjectNote(title, description, body *string) error {
	if title == nil || description == nil || body == nil {
		return fmt.Errorf("project note title, description, and body are required")
	}
	*title = strings.Join(strings.Fields(*title), " ")
	*description = strings.TrimSpace(*description)
	if *title == "" || *description == "" || strings.TrimSpace(*body) == "" {
		return fmt.Errorf("project note title, description, and body must not be blank")
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
		SELECT notes.id, notes.author_principal, principals.name, notes.title, notes.description, notes.created_at
		FROM gatehouse_project_notes AS notes
		JOIN gatehouse_principals AS principals ON principals.id = notes.author_principal
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
		var authorName sql.NullString
		note.Ref.Project = project
		if err := rows.Scan(&note.Ref.Id, &note.AuthorPrincipal.Id, &authorName, &note.Title, &note.Description, &note.CreatedAt); err != nil {
			return fmt.Errorf("scan project note: %w", err), nil
		}
		if authorName.Valid {
			note.AuthorName = &authorName.String
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
		SELECT notes.author_principal, principals.name, notes.title, notes.description, notes.body, notes.enabled, notes.created_at
		FROM gatehouse_project_notes AS notes
		JOIN gatehouse_principals AS principals ON principals.id = notes.author_principal
		WHERE notes.workspace = `+placeholder(1)+` AND notes.project = `+placeholder(2)+` AND notes.id = `+placeholder(3)+` AND notes.enabled = TRUE
	`, note.Project.Workspace.Id, note.Project.Id, note.Id)
	detail := &ProjectNoteDetail{Note: model.ProjectNote{Ref: note}}
	var authorName sql.NullString
	if err := row.Scan(&detail.Note.AuthorPrincipal.Id, &authorName, &detail.Note.Title, &detail.Note.Description, &detail.Note.Body, &detail.Note.Enabled, &detail.Note.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get project note: %w", err), nil
	}
	if authorName.Valid {
		detail.AuthorName = &authorName.String
	}
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
	note.AuthorPrincipal = principal
	note.Enabled = true
	note.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project note creation: %w", err), model.ProjectNote{}
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_project_notes (workspace, project, id, author_principal, title, description, body, enabled, created_at)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, TRUE, `+placeholder(8)+`)
	`, note.Ref.Project.Workspace.Id, note.Ref.Project.Id, note.Ref.Id, note.AuthorPrincipal.Id, note.Title, note.Description, note.Body, note.CreatedAt); err != nil {
		return fmt.Errorf("insert project note: %w", err), model.ProjectNote{}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{Workspace: note.Ref.Project.Workspace},
		Event:        "project_note.create",
		ResourceKind: ActivityResourceKindProject,
		Project:      &note.Ref.Project,
	}, []string{ActivityTopicProject(note.Ref.Project)}); err != nil {
		return fmt.Errorf("append project note creation activity: %w", err), model.ProjectNote{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project note creation: %w", err), model.ProjectNote{}
	}
	return nil, note
}

func (store *Store) ProjectNoteDetailsSet(ctx context.Context, note model.ProjectNoteRef, principal model.PrincipalRef, title, description, body *string) (error, *ProjectNoteDetail) {
	if err := normalizeProjectNote(title, description, body); err != nil {
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
	result, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_project_notes SET title = `+placeholder(1)+`, description = `+placeholder(2)+`, body = `+placeholder(3)+`
		WHERE workspace = `+placeholder(4)+` AND project = `+placeholder(5)+` AND id = `+placeholder(6)+` AND enabled = TRUE
	`, *title, *description, *body, note.Project.Workspace.Id, note.Project.Id, note.Id)
	if err != nil {
		return fmt.Errorf("update project note: %w", err), nil
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update project note: %w", err), nil
	}
	if changed != 1 {
		return nil, nil
	}
	detail.Note.Title, detail.Note.Description, detail.Note.Body = *title, *description, *body
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:          model.ActivityEventRef{Workspace: note.Project.Workspace},
		Event:        "project_note.update",
		ResourceKind: ActivityResourceKindProject,
		Project:      &note.Project,
	}, []string{ActivityTopicProject(note.Project)}); err != nil {
		return fmt.Errorf("append project note update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project note update: %w", err), nil
	}
	return nil, detail
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
		Ref:          model.ActivityEventRef{Workspace: note.Project.Workspace},
		Event:        "project_note.remove",
		ResourceKind: ActivityResourceKindProject,
		Project:      &note.Project,
	}, []string{ActivityTopicProject(note.Project)}); err != nil {
		return fmt.Errorf("append project note removal activity: %w", err), false
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project note removal: %w", err), false
	}
	return nil, true
}
