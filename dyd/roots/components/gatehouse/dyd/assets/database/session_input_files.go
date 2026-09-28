package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gatehouse/inputform"
	"gatehouse/model"
	"gatehouse/typed_id"
)

var ErrSessionInputFileUnavailable = errors.New("session input file is unavailable")

type sessionInputFileLink struct {
	Input model.SessionEventRef
	Path  []string
}

func inputFilePath(path []string) string {
	encoded, _ := json.Marshal(path)
	return string(encoded)
}

// SessionInputFileCreate creates a normal session file and ties it to one
// field descriptor of a pending input request. The file outlives the input.
func (store *Store) SessionInputFileCreate(ctx context.Context, input model.SessionEventRef, path []string, file model.SessionFile, objectID string, principal model.PrincipalRef) (error, model.SessionFile, string) {
	return store.sessionFileCreate(ctx, file, objectID, principal, &sessionInputFileLink{Input: input, Path: path})
}

func (store *Store) linkSessionInputFile(ctx context.Context, transaction *sql.Tx, input model.SessionEventRef, path []string, fileID string) error {
	placeholder := keychainPlaceholder(store.kind)
	result, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_session_input_files (workspace, session, input, field_path, file)
		SELECT `+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`
		WHERE EXISTS (SELECT 1 FROM gatehouse_session_events WHERE workspace = `+placeholder(6)+` AND session = `+placeholder(7)+` AND id = `+placeholder(8)+` AND kind = 'input.request')
		AND NOT EXISTS (SELECT 1 FROM gatehouse_session_input_responses WHERE workspace = `+placeholder(9)+` AND session = `+placeholder(10)+` AND input = `+placeholder(11)+`)
	`, input.Session.Workspace.Id, input.Session.Id, input.Id, inputFilePath(path), fileID,
		input.Session.Workspace.Id, input.Session.Id, input.Id, input.Session.Workspace.Id, input.Session.Id, input.Id)
	if err != nil {
		return fmt.Errorf("register session input file: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrSessionInputResolved
	}
	return nil
}

func (store *Store) inputFileSummary(ctx context.Context, transaction *sql.Tx, input model.SessionEventRef, path []string, id string) (error, inputform.FileSummary) {
	if !typed_id.Valid(typed_id.SessionFile, id) {
		return ErrSessionInputFileUnavailable, inputform.FileSummary{}
	}
	placeholder := keychainPlaceholder(store.kind)
	query := `SELECT files.name, files.media_type, objects.size
		FROM gatehouse_session_input_files AS links
		JOIN gatehouse_session_files AS files ON files.workspace = links.workspace AND files.session = links.session AND files.id = links.file
		JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object
		WHERE links.workspace = ` + placeholder(1) + ` AND links.session = ` + placeholder(2) + ` AND links.input = ` + placeholder(3) + `
			AND links.field_path = ` + placeholder(4) + ` AND links.file = ` + placeholder(5) + `
			AND files.enabled = TRUE AND objects.state = 'success'`
	var row *sql.Row
	if transaction == nil {
		row = store.QueryRowContext(ctx, query, input.Session.Workspace.Id, input.Session.Id, input.Id, inputFilePath(path), id)
	} else {
		row = transaction.QueryRowContext(ctx, query, input.Session.Workspace.Id, input.Session.Id, input.Id, inputFilePath(path), id)
	}
	file := inputform.FileSummary{ID: id}
	var mediaType sql.NullString
	if err := row.Scan(&file.Name, &mediaType, &file.Size); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSessionInputFileUnavailable, inputform.FileSummary{}
		}
		return fmt.Errorf("read session input file: %w", err), inputform.FileSummary{}
	}
	if mediaType.Valid {
		file.MediaType = &mediaType.String
	}
	return nil, file
}

func (store *Store) SessionInputFileSummaryGet(ctx context.Context, input model.SessionEventRef, path []string, id string) (error, inputform.FileSummary) {
	return store.inputFileSummary(ctx, nil, input, path, id)
}

func (store *Store) SessionInputFileGet(ctx context.Context, input model.SessionEventRef, path []string, id string, principal model.PrincipalRef) (error, *model.SessionFile, *StorageObject) {
	if !typed_id.Valid(typed_id.SessionFile, id) {
		return ErrSessionInputFileUnavailable, nil, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	var exists bool
	err := store.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM gatehouse_session_input_files
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND input = `+placeholder(3)+`
		AND field_path = `+placeholder(4)+` AND file = `+placeholder(5)+`)`,
		input.Session.Workspace.Id, input.Session.Id, input.Id, inputFilePath(path), id).Scan(&exists)
	if err != nil {
		return err, nil, nil
	}
	if !exists {
		return ErrSessionInputFileUnavailable, nil, nil
	}
	err, file, object := store.SessionFileGet(ctx, model.SessionFileRef{Session: input.Session, Id: id}, principal)
	return err, file, object
}

// CanonicalizeInputFiles verifies references and replaces client-provided
// metadata with the completed session file's stored name, size and media type.
func (store *Store) CanonicalizeInputFiles(ctx context.Context, input model.SessionEventRef, field inputform.Field, path []string, value json.RawMessage) (error, json.RawMessage) {
	return store.canonicalizeInputFiles(ctx, nil, input, field, path, value)
}

func (store *Store) canonicalizeInputFiles(ctx context.Context, transaction *sql.Tx, input model.SessionEventRef, field inputform.Field, path []string, value json.RawMessage) (error, json.RawMessage) {
	switch field.Type {
	case "files":
		var submitted []inputform.FileSummary
		if err := json.Unmarshal(value, &submitted); err != nil {
			return err, nil
		}
		canonical := make([]inputform.FileSummary, 0, len(submitted))
		for _, reference := range submitted {
			err, file := store.inputFileSummary(ctx, transaction, input, path, reference.ID)
			if err != nil {
				return err, nil
			}
			if !inputform.AllowsMediaType(field, mediaTypeValue(file.MediaType)) {
				return ErrSessionInputFileUnavailable, nil
			}
			canonical = append(canonical, file)
		}
		encoded, err := json.Marshal(canonical)
		return err, encoded
	case "object":
		var object map[string]json.RawMessage
		if err := json.Unmarshal(value, &object); err != nil || object == nil {
			return ErrSessionInputFileUnavailable, nil
		}
		for _, child := range field.Fields {
			if item, ok := object[child.ID]; ok {
				err, result := store.canonicalizeInputFiles(ctx, transaction, input, child, append(path, child.ID), item)
				if err != nil {
					return err, nil
				}
				object[child.ID] = result
			}
		}
		encoded, err := json.Marshal(object)
		return err, encoded
	case "list":
		var items []json.RawMessage
		if err := json.Unmarshal(value, &items); err != nil || items == nil {
			return ErrSessionInputFileUnavailable, nil
		}
		for index, item := range items {
			err, result := store.canonicalizeInputFiles(ctx, transaction, input, *field.Item, path, item)
			if err != nil {
				return err, nil
			}
			items[index] = result
		}
		encoded, err := json.Marshal(items)
		return err, encoded
	default:
		return nil, value
	}
}

func mediaTypeValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.ToLower(*value)
}
