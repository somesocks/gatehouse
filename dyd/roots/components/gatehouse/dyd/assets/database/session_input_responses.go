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

var ErrSessionInputResolved = errors.New("session input is already resolved")
var ErrSessionInputInvalidDraft = errors.New("invalid session input draft")

// SessionInputResponseCreate atomically validates the stored draft, writes one
// terminal child event, and deletes the draft. Workflow delivery must use
// SessionInputResponseCreateInTransaction to share the response transaction.
// A success payload is always taken from the stored draft, not the caller.
func (store *Store) SessionInputResponseCreate(ctx context.Context, event model.SessionEvent) (error, model.SessionEvent) {
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session input response: %w", err), model.SessionEvent{}
	}
	defer transaction.Rollback()
	err, stored := store.SessionInputResponseCreateInTransaction(ctx, transaction, event)
	if err != nil {
		return err, model.SessionEvent{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session input response: %w", err), model.SessionEvent{}
	}
	return nil, stored
}

// SessionInputCancellationCreate resolves a pending form whose tool workflow
// has already been cancelled, so no workflow message is needed.
func (store *Store) SessionInputCancellationCreate(ctx context.Context, event model.SessionEvent) (error, model.SessionEvent) {
	if event.Kind != model.SessionEventKindInputFailure || event.Payload["code"] != "cancelled" {
		return fmt.Errorf("create session input cancellation: event must be input.failure with code cancelled"), model.SessionEvent{}
	}
	return store.SessionInputResponseCreate(ctx, event)
}

// SessionInputResponseCreateInTransaction validates and persists the terminal
// response using the caller's transaction, which can also deliver its reference
// to DBOS before committing. The submitted values stay in Gatehouse events.
func (store *Store) SessionInputResponseCreateInTransaction(ctx context.Context, transaction *sql.Tx, event model.SessionEvent) (error, model.SessionEvent) {
	if transaction == nil {
		return fmt.Errorf("create session input response: transaction is required"), model.SessionEvent{}
	}
	if event.Parent == nil || *event.Parent == event.Ref || event.Parent.Session != event.Ref.Session ||
		event.AuthorPrincipal == nil || event.AuthorPrincipal.Ref.Id == "" || event.AuthorAgent != nil || event.AuthorGateway != nil ||
		(event.Kind != model.SessionEventKindInputSuccess && event.Kind != model.SessionEventKindInputFailure) || !typed_id.Valid(typed_id.SessionEvent, event.Ref.Id) {
		return fmt.Errorf("create session input response: invalid terminal event"), model.SessionEvent{}
	}
	if event.Kind == model.SessionEventKindInputSuccess && len(event.Payload) != 0 {
		return fmt.Errorf("create session input response: success must use the stored draft"), model.SessionEvent{}
	}
	if event.Kind == model.SessionEventKindInputFailure {
		code, ok := event.Payload["code"].(string)
		message, hasMessage := event.Payload["message"].(string)
		if !ok || !validInputFailureCode(code) || len(event.Payload) != 1 && !(len(event.Payload) == 2 && hasMessage) {
			return fmt.Errorf("create session input response: failure requires a valid code and optional message"), model.SessionEvent{}
		}
		if hasMessage && strings.TrimSpace(message) == "" {
			return fmt.Errorf("create session input response: failure message must not be blank"), model.SessionEvent{}
		}
	}
	err, draft := store.sessionInputDraftLock(ctx, transaction, *event.Parent)
	if err != nil {
		return err, model.SessionEvent{}
	}
	if draft == "" {
		return fmt.Errorf("create session input response: request is unavailable"), model.SessionEvent{}
	}
	placeholder := keychainPlaceholder(store.kind)
	var resolved bool
	if err := transaction.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM gatehouse_session_events
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND parent = `+placeholder(3)+`
			AND kind IN ('input.success', 'input.failure'))
	`, event.Ref.Session.Workspace.Id, event.Ref.Session.Id, event.Parent.Id).Scan(&resolved); err != nil {
		return fmt.Errorf("check session input resolution: %w", err), model.SessionEvent{}
	}
	if resolved {
		return ErrSessionInputResolved, model.SessionEvent{}
	}
	var requestPayload string
	err = transaction.QueryRowContext(ctx, `
		SELECT inputs.payload FROM gatehouse_session_events AS inputs
		JOIN gatehouse_session_events AS tools ON tools.workspace = inputs.workspace
			AND tools.session = inputs.session AND tools.id = inputs.parent
		WHERE inputs.workspace = `+placeholder(1)+` AND inputs.session = `+placeholder(2)+` AND inputs.id = `+placeholder(3)+`
			AND inputs.kind = '`+model.SessionEventKindInputRequest+`' AND inputs.author_agent IS NOT NULL
			AND tools.kind = '`+model.SessionEventKindToolRequest+`' AND tools.author_agent = inputs.author_agent
	`, event.Ref.Session.Workspace.Id, event.Ref.Session.Id, event.Parent.Id).Scan(&requestPayload)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("create session input response: invalid request parent"), model.SessionEvent{}
	}
	if err != nil {
		return fmt.Errorf("read session input request: %w", err), model.SessionEvent{}
	}
	if event.Kind == model.SessionEventKindInputSuccess {
		var request struct {
			Form json.RawMessage `json:"form"`
		}
		if err := json.Unmarshal([]byte(requestPayload), &request); err != nil {
			return fmt.Errorf("decode session input request: %w", err), model.SessionEvent{}
		}
		err, form := inputform.Decode(request.Form)
		if err != nil {
			return fmt.Errorf("validate session input form: %w", err), model.SessionEvent{}
		}
		if err := form.ValidateResult(json.RawMessage(draft)); err != nil {
			return fmt.Errorf("%w: %w", ErrSessionInputInvalidDraft, err), model.SessionEvent{}
		}
		var values map[string]json.RawMessage
		if err := json.Unmarshal([]byte(draft), &values); err != nil {
			return err, model.SessionEvent{}
		}
		for _, field := range form.Fields {
			if value, exists := values[field.ID]; exists {
				if err, _ := store.canonicalizeInputFiles(ctx, transaction, event.Ref.Session, field, value); err != nil {
					return fmt.Errorf("%w: %w", ErrSessionInputInvalidDraft, err), model.SessionEvent{}
				}
			}
		}
		event.Payload = map[string]interface{}{"result": json.RawMessage(draft)}
	}
	createdAt, err := typed_id.Timestamp(typed_id.SessionEvent, event.Ref.Id)
	if err != nil {
		return fmt.Errorf("create session input response: invalid event ID: %w", err), model.SessionEvent{}
	}
	event.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("encode session input response: %w", err), model.SessionEvent{}
	}
	result, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_session_events (
			workspace, session, id, parent, kind, author_principal, author_agent, author_gateway, payload, metrics, created_at
		) VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`,
			`+placeholder(6)+`, NULL, NULL, `+placeholder(7)+`, NULL, `+placeholder(8)+`)
		ON CONFLICT (workspace, session, parent) WHERE kind IN ('input.success', 'input.failure') DO NOTHING
	`, event.Ref.Session.Workspace.Id, event.Ref.Session.Id, event.Ref.Id, event.Parent.Id, event.Kind, event.AuthorPrincipal.Ref.Id, string(payload), event.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert session input response: %w", err), model.SessionEvent{}
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check session input response: %w", err), model.SessionEvent{}
	}
	if inserted != 1 {
		return ErrSessionInputResolved, model.SessionEvent{}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{
		Ref:                  model.ActivityEventRef{},
		Event:                "session_event.create",
		ResourceKind:         ActivityResourceKindSessionEvent,
		ResourceSessionEvent: &event.Ref.Id,
		CreatedAt:            event.CreatedAt,
	}, []string{ActivityTopicSessionEvent(event.Ref)}); err != nil {
		return fmt.Errorf("append session input response activity: %w", err), model.SessionEvent{}
	}
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM gatehouse_session_input_drafts
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND input = `+placeholder(3)+`
	`, event.Ref.Session.Workspace.Id, event.Ref.Session.Id, event.Parent.Id); err != nil {
		return fmt.Errorf("delete resolved session input draft: %w", err), model.SessionEvent{}
	}
	return nil, event
}

func validInputFailureCode(code string) bool {
	if code == "" || len(code) > 64 || code != strings.TrimSpace(code) {
		return false
	}
	for index, character := range code {
		if character >= 'a' && character <= 'z' || index > 0 && (character >= '0' && character <= '9' || character == '_' || character == '-') {
			continue
		}
		return false
	}
	return true
}

// SessionInputResponseResultGet preserves the exact JSON number text from the
// stored event instead of decoding it through the generic event map.
func (store *Store) SessionInputResponseResultGet(ctx context.Context, response model.SessionEventRef) (error, string) {
	placeholder := keychainPlaceholder(store.kind)
	var payload string
	err := store.QueryRowContext(ctx, `
		SELECT payload FROM gatehouse_session_events
		WHERE workspace = `+placeholder(1)+` AND session = `+placeholder(2)+` AND id = `+placeholder(3)+` AND kind = '`+model.SessionEventKindInputSuccess+`'
	`, response.Session.Workspace.Id, response.Session.Id, response.Id).Scan(&payload)
	if err != nil {
		return fmt.Errorf("read session input result: %w", err), ""
	}
	var content struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(payload), &content); err != nil || len(content.Result) == 0 {
		return fmt.Errorf("read session input result: invalid payload"), ""
	}
	return nil, string(content.Result)
}
