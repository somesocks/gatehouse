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

func (store *Store) inputFileSummary(ctx context.Context, transaction *sql.Tx, session model.SessionRef, id string) (error, inputform.FileSummary) {
	if !typed_id.Valid(typed_id.SessionFile, id) {
		return ErrSessionInputFileUnavailable, inputform.FileSummary{}
	}
	placeholder := keychainPlaceholder(store.kind)
	query := `SELECT files.name, files.media_type, objects.size
		FROM gatehouse_session_files AS files
		JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object
		WHERE files.workspace = ` + placeholder(1) + ` AND files.session = ` + placeholder(2) + ` AND files.id = ` + placeholder(3) + `
			AND files.enabled = TRUE AND objects.state = 'success'`
	var row *sql.Row
	if transaction == nil {
		row = store.QueryRowContext(ctx, query, session.Workspace.Id, session.Id, id)
	} else {
		row = transaction.QueryRowContext(ctx, query, session.Workspace.Id, session.Id, id)
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

// CanonicalizeInputFiles verifies references and replaces client-provided
// metadata with the completed session file's stored name, size and media type.
func (store *Store) CanonicalizeInputFiles(ctx context.Context, session model.SessionRef, field inputform.Field, value json.RawMessage) (error, json.RawMessage) {
	return store.canonicalizeInputFiles(ctx, nil, session, field, value)
}

func (store *Store) canonicalizeInputFiles(ctx context.Context, transaction *sql.Tx, session model.SessionRef, field inputform.Field, value json.RawMessage) (error, json.RawMessage) {
	switch field.Type {
	case "files":
		var submitted []inputform.FileSummary
		if err := json.Unmarshal(value, &submitted); err != nil {
			return err, nil
		}
		canonical := make([]inputform.FileSummary, 0, len(submitted))
		for _, reference := range submitted {
			err, file := store.inputFileSummary(ctx, transaction, session, reference.ID)
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
				err, result := store.canonicalizeInputFiles(ctx, transaction, session, child, item)
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
			err, result := store.canonicalizeInputFiles(ctx, transaction, session, *field.Item, item)
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
