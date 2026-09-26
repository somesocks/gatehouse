package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gatehouse/config"
	"gatehouse/model"
	"gatehouse/typed_id"
)

var ErrSessionInputDraftPath = errors.New("invalid session input draft path")

// SessionInputDraft is the mutable, shared form state for an input request.
// A missing row means that the draft is still the empty JSON object.
type SessionInputDraft struct {
	Request   model.SessionEventRef
	Values    json.RawMessage
	UpdatedAt string
}

func (store *Store) SessionInputDraftGet(ctx context.Context, request model.SessionEventRef) (error, *SessionInputDraft) {
	placeholder := keychainPlaceholder(store.kind)
	var value SessionInputDraft
	value.Request = request
	var encoded string
	err := store.QueryRowContext(ctx, `
		SELECT draft, updated_at FROM gatehouse_session_input_drafts
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND input = `+placeholder(3)+`
	`, request.Session.Workspace.Id, request.Session.Id, request.Id).Scan(&encoded, &value.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("get session input draft: %w", err), nil
	}
	value.Values = json.RawMessage(encoded)
	return nil, &value
}

// SessionInputDraftSet replaces one field or subtree, including an entire
// array-valued field. It never treats array indices as path components.
func (store *Store) SessionInputDraftSet(ctx context.Context, request model.SessionEventRef, path []string, value json.RawMessage) (error, *SessionInputDraft) {
	if !json.Valid(value) {
		return fmt.Errorf("set session input draft: value must be valid JSON"), nil
	}
	return store.sessionInputDraftPatch(ctx, request, path, value)
}

// SessionInputDraftRemove removes one field or subtree. An absent field is a
// no-op; JSON null must be set explicitly through SessionInputDraftSet.
func (store *Store) SessionInputDraftRemove(ctx context.Context, request model.SessionEventRef, path []string) (error, *SessionInputDraft) {
	return store.sessionInputDraftPatch(ctx, request, path, nil)
}

func (store *Store) sessionInputDraftPatch(ctx context.Context, request model.SessionEventRef, path []string, value json.RawMessage) (error, *SessionInputDraft) {
	if !typed_id.Valid(typed_id.SessionEvent, request.Id) {
		return fmt.Errorf("patch session input draft: invalid request ID"), nil
	}
	if len(path) == 0 {
		return ErrSessionInputDraftPath, nil
	}
	for _, segment := range path {
		if strings.TrimSpace(segment) == "" {
			return ErrSessionInputDraftPath, nil
		}
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session input draft patch: %w", err), nil
	}
	defer transaction.Rollback()

	placeholder := keychainPlaceholder(store.kind)
	updatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	// The insert acquires SQLite's writer lock before the draft is read. For
	// PostgreSQL the subsequent SELECT FOR UPDATE locks the draft row. Both
	// paths serialize read/modify/write operations without a revision column.
	_, err = transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_session_input_drafts (workspace, session, input, draft, updated_at)
		SELECT workspace, session, id, '{}', `+placeholder(1)+`
		FROM gatehouse_session_events
		WHERE workspace = `+placeholder(2)+` AND session = `+placeholder(3)+` AND id = `+placeholder(4)+`
			AND kind = 'input.request' AND author_agent IS NOT NULL
		ON CONFLICT (workspace, session, input) DO NOTHING
	`, updatedAt, request.Session.Workspace.Id, request.Session.Id, request.Id)
	if err != nil {
		return fmt.Errorf("initialize session input draft: %w", err), nil
	}
	query := `
		SELECT draft FROM gatehouse_session_input_drafts
		WHERE workspace = ` + placeholder(1) + ` AND session = ` + placeholder(2) + ` AND input = ` + placeholder(3)
	if store.kind == config.DatabaseKindPostgres {
		query += " FOR UPDATE"
	}
	var raw string
	err = transaction.QueryRowContext(ctx, query, request.Session.Workspace.Id, request.Session.Id, request.Id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("read session input draft for patch: %w", err), nil
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &document); err != nil || document == nil {
		return fmt.Errorf("decode session input draft: invalid JSON object"), nil
	}
	if err := patchInputDraftObject(document, path, value); err != nil {
		return err, nil
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode session input draft: %w", err), nil
	}
	updatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	updateValue := placeholder(1)
	if store.kind == config.DatabaseKindPostgres {
		updateValue += "::jsonb"
	}
	_, err = transaction.ExecContext(ctx, `
		UPDATE gatehouse_session_input_drafts
		SET draft = `+updateValue+`, updated_at = `+placeholder(2)+`
		WHERE workspace = `+placeholder(3)+` AND session = `+placeholder(4)+` AND input = `+placeholder(5)+`
	`, string(encoded), updatedAt, request.Session.Workspace.Id, request.Session.Id, request.Id)
	if err != nil {
		return fmt.Errorf("update session input draft: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session input draft patch: %w", err), nil
	}
	return nil, &SessionInputDraft{Request: request, Values: encoded, UpdatedAt: updatedAt}
}

func patchInputDraftObject(document map[string]json.RawMessage, path []string, value json.RawMessage) error {
	key := path[0]
	if len(path) == 1 {
		if value == nil {
			delete(document, key)
		} else {
			document[key] = append(json.RawMessage(nil), value...)
		}
		return nil
	}
	var nested map[string]json.RawMessage
	if current, exists := document[key]; exists {
		if err := json.Unmarshal(current, &nested); err != nil || nested == nil {
			return ErrSessionInputDraftPath
		}
	} else {
		if value == nil {
			return nil
		}
		nested = make(map[string]json.RawMessage)
	}
	if err := patchInputDraftObject(nested, path[1:], value); err != nil {
		return err
	}
	encoded, err := json.Marshal(nested)
	if err != nil {
		return fmt.Errorf("encode nested session input draft: %w", err)
	}
	document[key] = encoded
	return nil
}
