package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"gatehouse/inputform"
	"gatehouse/model"
)

// SessionInputRequestFormGet reads the frozen descriptor without converting
// JSON numbers through float64. Only an agent input attached to its own tool
// request is a valid input request.
func (store *Store) SessionInputRequestFormGet(ctx context.Context, request model.SessionEventRef) (error, *inputform.Form) {
	placeholder := keychainPlaceholder(store.kind)
	var payload string
	err := store.QueryRowContext(ctx, `
		SELECT inputs.payload FROM gatehouse_session_events AS inputs
		JOIN gatehouse_session_events AS tools ON tools.workspace = inputs.workspace
			AND tools.session = inputs.session AND tools.id = inputs.parent
		WHERE inputs.workspace = `+placeholder(1)+` AND inputs.session = `+placeholder(2)+` AND inputs.id = `+placeholder(3)+`
			AND inputs.kind = '`+model.SessionEventKindInputRequest+`' AND inputs.author_agent IS NOT NULL
			AND tools.kind = '`+model.SessionEventKindToolRequest+`' AND tools.author_agent = inputs.author_agent
	`, request.Session.Workspace.Id, request.Session.Id, request.Id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("read session input request: %w", err), nil
	}
	var content struct {
		Form json.RawMessage `json:"form"`
	}
	if err := json.Unmarshal([]byte(payload), &content); err != nil {
		return fmt.Errorf("decode session input request: %w", err), nil
	}
	err, form := inputform.Decode(content.Form)
	if err != nil {
		return fmt.Errorf("decode session input form: %w", err), nil
	}
	return nil, form
}

// SessionInputRequestToolGetInTransaction resolves the validated request's
// owning tool so its response can be sent in the same transaction as the event.
func (store *Store) SessionInputRequestToolGetInTransaction(ctx context.Context, transaction *sql.Tx, request model.SessionEventRef) (error, model.SessionEventRef) {
	if transaction == nil {
		return fmt.Errorf("read session input tool: transaction is required"), model.SessionEventRef{}
	}
	placeholder := keychainPlaceholder(store.kind)
	tool := model.SessionEventRef{Session: request.Session}
	err := transaction.QueryRowContext(ctx, `
		SELECT tools.id FROM gatehouse_session_events AS inputs
		JOIN gatehouse_session_events AS tools ON tools.workspace = inputs.workspace
			AND tools.session = inputs.session AND tools.id = inputs.parent
		WHERE inputs.workspace = `+placeholder(1)+` AND inputs.session = `+placeholder(2)+` AND inputs.id = `+placeholder(3)+`
			AND inputs.kind = '`+model.SessionEventKindInputRequest+`' AND inputs.author_agent IS NOT NULL
			AND tools.kind = '`+model.SessionEventKindToolRequest+`' AND tools.author_agent = inputs.author_agent
	`, request.Session.Workspace.Id, request.Session.Id, request.Id).Scan(&tool.Id)
	if err != nil {
		return fmt.Errorf("read session input tool: %w", err), model.SessionEventRef{}
	}
	return nil, tool
}

func (store *Store) SessionInputResolvedGet(ctx context.Context, request model.SessionEventRef) (error, bool) {
	placeholder := keychainPlaceholder(store.kind)
	var resolved bool
	err := store.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM gatehouse_session_events
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND parent = `+placeholder(3)+`
			AND kind IN ('input.success', 'input.failure'))
	`, request.Session.Workspace.Id, request.Session.Id, request.Id).Scan(&resolved)
	if err != nil {
		return fmt.Errorf("check session input response: %w", err), false
	}
	return nil, resolved
}

// SessionInputStateGet reads resolution and the draft in one SQL statement.
// Its snapshot cannot show a deleted draft alongside the old pending state
// while another responder is submitting or cancelling the input.
type SessionInputState struct {
	Draft     json.RawMessage
	UpdatedAt *string
	Resolved  bool
}

func (store *Store) SessionInputStateGet(ctx context.Context, request model.SessionEventRef) (error, *SessionInputState) {
	placeholder := keychainPlaceholder(store.kind)
	var draft, updatedAt, response sql.NullString
	err := store.QueryRowContext(ctx, `
		SELECT drafts.draft, drafts.updated_at, responses.id
		FROM gatehouse_session_events AS inputs
		JOIN gatehouse_session_events AS tools ON tools.workspace = inputs.workspace
			AND tools.session = inputs.session AND tools.id = inputs.parent
		LEFT JOIN gatehouse_session_input_drafts AS drafts ON drafts.workspace = inputs.workspace
			AND drafts.session = inputs.session AND drafts.input = inputs.id
		LEFT JOIN gatehouse_session_events AS responses ON responses.workspace = inputs.workspace
			AND responses.session = inputs.session AND responses.parent = inputs.id
			AND responses.kind IN ('input.success', 'input.failure')
		WHERE inputs.workspace = `+placeholder(1)+` AND inputs.session = `+placeholder(2)+` AND inputs.id = `+placeholder(3)+`
			AND inputs.kind = '`+model.SessionEventKindInputRequest+`' AND inputs.author_agent IS NOT NULL
			AND tools.kind = '`+model.SessionEventKindToolRequest+`' AND tools.author_agent = inputs.author_agent
	`, request.Session.Workspace.Id, request.Session.Id, request.Id).Scan(&draft, &updatedAt, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("read session input state: %w", err), nil
	}
	state := &SessionInputState{Draft: json.RawMessage(`{}`), Resolved: response.Valid}
	if draft.Valid {
		state.Draft = json.RawMessage(draft.String)
		state.UpdatedAt = &updatedAt.String
	}
	return nil, state
}
