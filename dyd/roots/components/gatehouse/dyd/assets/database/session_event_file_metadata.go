package database

import (
	"context"
	"database/sql"
	"fmt"
	"maps"

	"gatehouse/model"
	"gatehouse/typed_id"
)

// SessionEventFilesHydrateInTransaction builds a transient metadata projection
// from canonical attachment IDs and output_file references. It never reads file
// contents, opens another transaction, or mutates the input payload. Historical
// references remain renderable after a file is disabled; this is not permission
// to read its body. The caller supplies a transaction from this store.
func (store *Store) SessionEventFilesHydrateInTransaction(ctx context.Context, tx *sql.Tx, event model.SessionEvent) (error, model.SessionEvent) {
	if tx == nil || event.Ref.Session.Workspace.Id == "" || event.Ref.Session.Id == "" {
		return fmt.Errorf("hydrate event files: invalid transaction or session scope"), model.SessionEvent{}
	}
	if err := ctx.Err(); err != nil {
		return err, model.SessionEvent{}
	}
	_, attachmentsPresent := event.Payload["attachments"]
	_, outputPresent := event.Payload["output_file"]
	if !attachmentsPresent && !outputPresent {
		return nil, event
	}
	payload := maps.Clone(event.Payload)
	files := make(map[string]map[string]interface{})
	get := func(id string) (map[string]interface{}, error) {
		if !typed_id.Valid(typed_id.SessionFile, id) {
			return nil, fmt.Errorf("invalid session file ID %q", id)
		}
		if value, exists := files[id]; exists {
			return value, nil
		}
		p := keychainPlaceholder(store.kind)
		var name string
		var mediaType sql.NullString
		var size int64
		err := tx.QueryRowContext(ctx, `SELECT files.name, files.media_type, objects.size
 FROM gatehouse_session_files AS files
 JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object
 WHERE files.workspace = `+p(1)+` AND files.session = `+p(2)+` AND files.id = `+p(3),
			event.Ref.Session.Workspace.Id, event.Ref.Session.Id, id).Scan(&name, &mediaType, &size)
		if err != nil {
			return nil, fmt.Errorf("get session file metadata %q: %w", id, err)
		}
		if name == "" || size < 0 {
			return nil, fmt.Errorf("invalid session file metadata %q", id)
		}
		value := map[string]interface{}{"id": id, "name": name, "size": size}
		if mediaType.Valid {
			value["media_type"] = mediaType.String
		}
		files[id] = value
		return value, nil
	}
	if _, exists := payload["attachments"]; exists {
		ids, linked, err := sessionEventAttachmentIDs(event)
		if err != nil {
			return err, model.SessionEvent{}
		}
		if !linked {
			return fmt.Errorf("hydrate event files: attachments must be string IDs"), model.SessionEvent{}
		}
		attachments := make([]interface{}, 0, len(ids))
		for _, id := range ids {
			value, err := get(id)
			if err != nil {
				return err, model.SessionEvent{}
			}
			attachments = append(attachments, value)
		}
		payload["attachments"] = attachments
	}
	ref, present, err := SessionEventOutputFile(event)
	if err != nil {
		return err, model.SessionEvent{}
	}
	if present {
		value, err := get(ref.Id)
		if err != nil {
			return err, model.SessionEvent{}
		}
		payload["output_file"] = value
	}
	event.Payload = payload
	return nil, event
}
