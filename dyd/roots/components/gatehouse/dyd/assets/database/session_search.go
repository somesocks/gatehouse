package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gatehouse/config"
	"gatehouse/model"
	"gatehouse/sessionsearch"
	"gatehouse/typed_id"
)

// SessionEventsSearch returns one newest-first page of matching session events.
func (store *Store) SessionEventsSearch(ctx context.Context, session model.SessionRef, expression sessionsearch.Expression, beforeID string, limit int) (error, []model.SessionEvent, bool) {
	if limit < 1 {
		return fmt.Errorf("search session events: limit must be positive"), nil, false
	}
	if beforeID != "" && !typed_id.Valid(typed_id.SessionEvent, beforeID) {
		return fmt.Errorf("search session events: invalid cursor"), nil, false
	}
	placeholder := keychainPlaceholder(store.kind)
	arguments := []any{session.Workspace.Id, session.Id}
	where := []string{
		"events.workspace = " + placeholder(len(arguments)-1),
		"events.session = " + placeholder(len(arguments)),
	}
	if beforeID != "" {
		arguments = append(arguments, beforeID)
		where = append(where, "events.id < "+placeholder(len(arguments)))
	}
	predicate, arguments := sessionEventSearchPredicate(store.kind, expression, placeholder, arguments)
	where = append(where, predicate)
	arguments = append(arguments, limit+1)
	query := `
		SELECT events.id, events.kind, events.payload
		FROM gatehouse_session_events AS events
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY events.id DESC
		LIMIT ` + placeholder(len(arguments))
	rows, err := store.QueryContext(ctx, query, arguments...)
	if err != nil {
		return fmt.Errorf("search session events: %w", err), nil, false
	}
	defer rows.Close()
	results := make([]model.SessionEvent, 0, limit)
	for rows.Next() {
		var id, kind, payload string
		if err := rows.Scan(&id, &kind, &payload); err != nil {
			return fmt.Errorf("scan session event search result: %w", err), nil, false
		}
		decoded := map[string]interface{}{}
		if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
			return fmt.Errorf("decode session event search payload: %w", err), nil, false
		}
		results = append(results, model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: id}, Kind: kind, Payload: decoded})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate session event search results: %w", err), nil, false
	}
	more := len(results) > limit
	if more {
		results = results[:limit]
	}
	return nil, results, more
}

func sessionEventSearchPredicate(kind config.DatabaseKind, expression sessionsearch.Expression, placeholder func(int) string, arguments []any) (string, []any) {
	if expression.Operator == "" {
		if kind == config.DatabaseKindSQLite {
			return sessionEventSearchSQLiteTextPredicate(expression.Text, placeholder, arguments)
		}
		arguments = append(arguments, expression.Text)
		parameter := placeholder(len(arguments))
		return sessionEventSearchTextPredicate(parameter), arguments
	}
	children := make([]string, 0, len(expression.Children))
	for _, child := range expression.Children {
		predicate, updated := sessionEventSearchPredicate(kind, child, placeholder, arguments)
		arguments = updated
		children = append(children, predicate)
	}
	return "(" + strings.Join(children, " "+strings.ToUpper(expression.Operator)+" ") + ")", arguments
}

func sessionEventSearchSQLiteTextPredicate(text string, placeholder func(int) string, arguments []any) (string, []any) {
	contains := func(value string) string {
		arguments = append(arguments, text)
		return "instr(lower(COALESCE(" + value + ", '')), lower(" + placeholder(len(arguments)) + ")) > 0"
	}
	textValue := "json_extract(events.payload, '$.text')"
	outputValue := "json_extract(events.payload, '$.output')"
	codeValue := "json_extract(events.payload, '$.code')"
	reasonValue := "json_extract(events.payload, '$.reason')"
	descriptionValue := "json_extract(events.payload, '$.description')"
	message := contains(textValue)
	output := contains(outputValue)
	request := contains(codeValue + " || char(10) || " + reasonValue)
	approval := contains(descriptionValue)
	return "((events.kind = 'message.text' AND json_type(events.payload, '$.text') = 'text' AND " + message + ") OR " +
		"(events.kind IN ('tool.success', 'tool.failure') AND json_type(events.payload, '$.output') = 'text' AND " + output + ") OR " +
		"(events.kind = 'tool.request' AND json_type(events.payload, '$.name') = 'text' AND " +
		"json_extract(events.payload, '$.name') = 'lisp' AND json_type(events.payload, '$.call_id') = 'text' AND " +
		"trim(COALESCE(json_extract(events.payload, '$.call_id'), '')) <> '' AND json_type(events.payload, '$.code') = 'text' AND " +
		"trim(COALESCE(" + codeValue + ", '')) <> '' AND json_type(events.payload, '$.reason') = 'text' AND " +
		"trim(COALESCE(" + reasonValue + ", '')) <> '' AND " + request + ") OR " +
		"(events.kind = 'approval.request' AND json_type(events.payload, '$.description') = 'text' AND " + approval + "))", arguments
}

func sessionEventSearchTextPredicate(parameter string) string {
	contains := func(value string) string {
		return "POSITION(lower(" + parameter + ") IN lower(COALESCE(" + value + ", ''))) > 0"
	}
	textValue := "events.payload ->> 'text'"
	outputValue := "events.payload ->> 'output'"
	codeValue := "events.payload ->> 'code'"
	reasonValue := "events.payload ->> 'reason'"
	descriptionValue := "events.payload ->> 'description'"
	message := contains(textValue)
	output := contains(outputValue)
	request := contains(codeValue + " || E'\\n' || " + reasonValue)
	approval := contains(descriptionValue)
	return "((events.kind = 'message.text' AND jsonb_typeof(events.payload -> 'text') = 'string' AND " + message + ") OR " +
		"(events.kind IN ('tool.success', 'tool.failure') AND jsonb_typeof(events.payload -> 'output') = 'string' AND " + output + ") OR " +
		"(events.kind = 'tool.request' AND jsonb_typeof(events.payload -> 'name') = 'string' AND " +
		"(events.payload ->> 'name') = 'lisp' AND jsonb_typeof(events.payload -> 'call_id') = 'string' AND " +
		"btrim(COALESCE(events.payload ->> 'call_id', '')) <> '' AND jsonb_typeof(events.payload -> 'code') = 'string' AND " +
		"btrim(COALESCE(" + codeValue + ", '')) <> '' AND jsonb_typeof(events.payload -> 'reason') = 'string' AND " +
		"btrim(COALESCE(" + reasonValue + ", '')) <> '' AND " + request + ") OR " +
		"(events.kind = 'approval.request' AND jsonb_typeof(events.payload -> 'description') = 'string' AND " + approval + "))"
}
