package database_test

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"testing"
	"time"

	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestPostgresTADWToolingCursorIntegration(t *testing.T) {
	store := newPostgresIntegrationTestStore(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	store.SetMaxOpenConns(1)
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: newTypedID(t, typed_id.Workspace)}, Id: newTypedID(t, typed_id.Session)}
	principal := newTypedID(t, typed_id.Principal)
	provider := newTypedID(t, typed_id.AgentProvider)
	agentModel := newTypedID(t, typed_id.AgentModel)
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Id: newTypedID(t, typed_id.WorkspaceAgent)}
	other := newTypedID(t, typed_id.WorkspaceAgent)
	at := time.Date(2026, 1, 1, 0, 0, 0, 123456000, time.UTC)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO gatehouse_workspaces (id, alias, enabled) VALUES ($1, 'tooling', TRUE)`, session.Workspace.Id)
	exec(`INSERT INTO gatehouse_principals (id, alias, revision, enabled) VALUES ($1, 'reader', 1, TRUE)`, principal)
	exec(`INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled, created_at) VALUES ($1, $2, $3, TRUE, $4)`, session.Workspace.Id, session.Id, principal, at)
	exec(`INSERT INTO gatehouse_agent_providers (id, alias, revision, protocol, enabled) VALUES ($1, 'tooling', 1, 'builtin', TRUE)`, provider)
	exec(`INSERT INTO gatehouse_agent_models (id, alias, revision, provider_id, model, parameters, enabled)
 VALUES ($1, 'tooling', 1, $2, 'dummy.fixed-reply', '{"text":"done"}', TRUE)`, agentModel, provider)
	exec(`INSERT INTO gatehouse_workspace_agents (workspace_id, id, alias, model_id, revision, enabled)
 VALUES ($1, $2, 'owner', $4, 1, TRUE), ($1, $3, 'other', $4, 1, TRUE)`, session.Workspace.Id, agent.Id, other, agentModel)

	id := func(n int) string { return fmt.Sprintf("sev_%026d", n) }
	// Exclude nested requests (30/31), foreign-agent subtrees (35/36), other
	// request branches (2/45), and IDs after thinking 60 even when backdated.
	nodes := []struct {
		id, parent, second int
		kind               string
	}{
		{1, 0, 0, "message.text"},
		{2, 1, 2, "agent.request"},
		{10, 1, 10, "agent.request"},
		{15, 10, 15, "thinking.request"},
		{20, 10, 20, "tool.request"},
		{25, 10, 20, "tool.request"},
		{30, 10, 30, "agent.request"},
		{31, 30, 31, "tool.request"},
		{35, 10, 35, "thinking.success"},
		{36, 35, 36, "tool.request"},
		{40, 20, 40, "tool.success"},
		{45, 2, 45, "tool.request"},
		{50, 25, 50, "tool.failure"},
		{55, 10, 55, "approval.request"},
		{56, 55, 56, "input.request"},
		{57, 56, 57, "tool.success"},
		{60, 10, 60, "thinking.request"},
		{70, 10, 5, "tool.request"},
		{71, 70, 71, "tool.success"},
		{80, 10, 80, "thinking.request"},
	}
	for _, node := range nodes {
		var parent, authorPrincipal, authorAgent any
		if node.parent != 0 {
			parent = id(node.parent)
		}
		payload := `{}`
		if node.kind == "message.text" || node.kind == "agent.request" {
			authorPrincipal = principal
			if node.kind == "agent.request" {
				payload = fmt.Sprintf(`{"agent":%q}`, agent.Id)
			}
		} else {
			authorAgent = agent.Id
			if node.id == 35 {
				authorAgent = other
			}
		}
		exec(`INSERT INTO gatehouse_session_events (workspace, session, id, parent, kind, author_principal, author_agent, payload, created_at)
 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, session.Workspace.Id, session.Id, id(node.id), parent, node.kind, authorPrincipal, authorAgent, payload, at.Add(time.Duration(node.second)*time.Second))
	}
	request := model.SessionEventRef{Session: session, Id: id(10)}
	read := func(tx *sql.Tx, thinking int, options database.TADWToolingReadOptions, limit int, want ...int) string {
		t.Helper()
		err, cursor := store.SessionTADWToolingCursor(ctx, tx, request, agent, model.SessionEventRef{Session: session, Id: id(thinking)}, options)
		if err != nil {
			t.Fatal(err)
		}
		defer cursor.Close()
		if cursor.RequestEvent().Ref != request {
			t.Fatal("cursor lost its owning request")
		}
		var got []string
		for (limit == 0 || len(got) < limit) && cursor.Next() {
			event := cursor.Event()
			got = append(got, event.Ref.Id)
			if event.Ref.Id == id(57) && (event.Parent == nil || event.Parent.Id != id(56)) {
				t.Fatal("tool result lost its actual parent across omitted bookkeeping")
			}
		}
		if err := cursor.Err(); err != nil {
			t.Fatal(err)
		}
		if err := cursor.Close(); err != nil {
			t.Fatal(err)
		}
		var expected []string
		for _, n := range want {
			expected = append(expected, id(n))
		}
		if !slices.Equal(got, expected) {
			t.Fatalf("thinking %d, options %+v, limit %d: tooling = %v, want %v", thinking, options, limit, got, expected)
		}
		return got[len(got)-1]
	}

	tx, err := store.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	read(tx, 60, database.TADWToolingReadOptions{}, 0, 10, 20, 25, 40, 50, 57)
	read(tx, 60, database.TADWToolingReadOptions{}, 2, 10, 20)
	boundary := read(tx, 60, database.TADWToolingReadOptions{Reverse: true}, 4, 57, 50, 40, 25)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Only an event ID crosses preparations, never a cursor or transaction.
	tx, err = store.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	read(tx, 60, database.TADWToolingReadOptions{FromEventID: boundary}, 0, 10, 25, 40, 50, 57)
	read(tx, 60, database.TADWToolingReadOptions{FromEventID: boundary, Reverse: true}, 0, 57, 50, 40, 25, 10)
	// The retained result 71 is reachable through ancestor 70 before the cutoff.
	read(tx, 80, database.TADWToolingReadOptions{FromEventID: id(40)}, 0, 10, 40, 50, 57, 71)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
