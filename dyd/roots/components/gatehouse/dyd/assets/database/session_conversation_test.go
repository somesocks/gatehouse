package database_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func TestConversationBranch(t *testing.T) {
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(t.Context(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	testConversationBranch(t, store, configuration)
}

func testConversationBranch(t *testing.T, store *database.Store, configuration config.DatabaseConfig) {
	t.Helper()
	ctx := t.Context()
	state := config.State{
		Workspaces:      []config.Workspace{{Alias: "conversation", Enabled: true}},
		Principals:      []config.Principal{{Alias: "reader", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "conversation", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Hello"}`, MaxTurns: 1, MaxOutputTokens: 1024, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "conversation", Alias: "first", ModelAlias: "conversation", Revision: 1, Enabled: true}, {WorkspaceID: "conversation", Alias: "second", ModelAlias: "conversation", Revision: 1, Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "conversation")
	var person model.PrincipalRef
	if err := store.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias=$1`, "reader").Scan(&person.Id); err != nil {
		t.Fatal(err)
	}
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &person, Enabled: true}, person); err != nil {
		t.Fatal(err)
	}
	var first, second string
	for alias, dest := range map[string]*string{"first": &first, "second": &second} {
		if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE workspace_id=$1 AND alias=$2`, workspace.Id, alias).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	refs := map[string]model.SessionEventRef{}
	sequence := 0
	add := func(name, parent, kind, agent string, payload map[string]any, at int) model.SessionEventRef {
		t.Helper()
		sequence++
		event := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: fmt.Sprintf("sev_%026d", sequence)}, Kind: kind, Payload: payload, CreatedAt: fmt.Sprintf("2026-01-01T00:00:%02d.000Z", at)}
		if parent != "" {
			ref := refs[parent]
			event.Parent = &ref
		}
		if agent == "" {
			event.AuthorPrincipal = &model.Principal{Ref: person, Enabled: true}
		} else {
			event.AuthorAgent = &model.WorkspaceAgentRef{Workspace: workspace, Id: agent}
		}
		if err, _ := store.SessionEventsCreate(ctx, event); err != nil {
			t.Fatal(err)
		}
		refs[name] = event.Ref
		return event.Ref
	}
	text := map[string]any{"text": "message"}
	request := func(bot string) map[string]any { return map[string]any{"agent": bot} }
	form := map[string]any{"description": "Review", "form": map[string]any{"private": "descriptor"}}
	add("a", "", "message.text", "", text, 0)
	add("ar", "a", "agent.request", "", request(first), 5)
	add("human", "a", "message.text", "", text, 5)
	add("third", "a", "agent.request", "", request(second), 5)
	add("third-answer", "third", "agent.success", second, text, 20)
	add("grandchild", "human", "agent.request", "", request(first), 6)
	add("grandchild-answer", "grandchild", "agent.success", first, text, 20)
	add("at", "ar", "tool.request", first, map[string]any{"code": "private"}, 10)
	add("ai", "at", "input.request", first, form, 11)
	add("ais", "ai", "input.success", "", map[string]any{"result": map[string]any{"private": "answer"}}, 12)
	add("answer", "ar", "agent.success", first, text, 20)
	add("answer-child", "answer", "message.text", "", text, 25)
	add("b", "", "message.text", "", text, 0) // Same-time root, higher ID.
	add("br", "b", "agent.request", "", request(second), 5)
	add("bt", "br", "tool.request", second, map[string]any{}, 10)
	add("bi", "bt", "input.request", second, form, 11)
	add("bapproval", "bt", "approval.request", second, map[string]any{"description": "Allow?"}, 12)
	// Wrong-agent and nested execution are excluded from br's interactions.
	add("foreign-tool", "br", "tool.request", first, map[string]any{}, 13)
	add("foreign-prompt", "foreign-tool", "input.request", first, form, 14)
	add("nested", "br", "agent.request", "", request(second), 13)
	add("nested-tool", "nested", "tool.request", second, map[string]any{}, 14)
	add("nested-prompt", "nested-tool", "approval.request", second, map[string]any{"description": "Nested"}, 15)
	for i, name := range []string{"c", "d", "e", "f"} {
		add(name, "", "message.text", "", text, i+1)
		add(name+"r", name, "agent.request", "", request(first), 5)
	}
	add("failed", "cr", "agent.failure", first, map[string]any{"code": "failed"}, 20)
	add("cancel", "dr", "cancel.request", "", map[string]any{}, 15)
	add("cancelled", "fr", "agent.failure", first, map[string]any{"code": "cancelled"}, 20)

	get := func(input database.ConversationBranchQuery) *database.ConversationBranch {
		t.Helper()
		err, page := store.SessionConversationBranchGet(ctx, session, input)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	page := get(database.ConversationBranchQuery{Limit: 1})
	if len(page.Entries) != 1 || page.Entries[0].Event.Ref != refs["a"] || page.Entries[0].HasMoreReplies || page.NextCursor == "" {
		t.Fatalf("first page = %#v", page)
	}
	previews := page.Entries[0].Previews
	if len(previews) != 3 || previews[0].Ref != refs["ar"] || previews[1].Ref != refs["human"] || previews[2].Ref != refs["third"] {
		t.Fatalf("previews = %#v", previews)
	}
	if len(page.AgentRequests) != 2 || page.AgentRequests[0].Request != refs["ar"] || page.AgentRequests[0].State != "completed" || page.AgentRequests[0].Outcome == nil || page.AgentRequests[0].Outcome.Ref != refs["answer"] || page.AgentRequests[1].Request != refs["third"] || page.AgentRequests[1].State != "completed" || page.AgentRequests[1].Outcome == nil || page.AgentRequests[1].Outcome.Ref != refs["third-answer"] {
		t.Fatalf("agent projection = %#v", page.AgentRequests)
	}
	if len(page.Interactions) != 1 || page.Interactions[0].Request.Ref != refs["ai"] || page.Interactions[0].Response == nil || page.Interactions[0].Response.Ref != refs["ais"] {
		t.Fatalf("resolved input = %#v", page.Interactions)
	}
	// Exactly three replies need no indicator. A fourth only sets the flag:
	// it must not be hydrated or expanded into an agent request/outcome.
	add("hidden", "a", "agent.request", "", request(second), 5)
	add("hidden-answer", "hidden", "agent.success", second, text, 20)
	more := get(database.ConversationBranchQuery{Limit: 1})
	if !more.Entries[0].HasMoreReplies || !reflect.DeepEqual(more.Entries[0].Previews, previews) || !reflect.DeepEqual(more.AgentRequests, page.AgentRequests) || !reflect.DeepEqual(more.Interactions, page.Interactions) {
		t.Fatalf("fourth reply changed visible projection: %#v", more)
	}
	next := get(database.ConversationBranchQuery{Limit: 1, Cursor: page.NextCursor})
	if next.Entries[0].Event.Ref != refs["b"] || len(next.AgentRequests) != 1 || next.AgentRequests[0].State != "waiting" || len(next.Interactions) != 2 {
		t.Fatalf("next page = %#v", next)
	}
	all := get(database.ConversationBranchQuery{Limit: 100})
	states := map[string]string{}
	for _, agent := range all.AgentRequests {
		states[agent.Request.Id] = agent.State
	}
	want := map[string]string{refs["ar"].Id: "completed", refs["third"].Id: "completed", refs["br"].Id: "waiting", refs["cr"].Id: "failed", refs["dr"].Id: "cancelling", refs["er"].Id: "working", refs["fr"].Id: "cancelled"}
	if !reflect.DeepEqual(states, want) || all.NextCursor != "" {
		t.Fatalf("states = %v, cursor %q", states, all.NextCursor)
	}
	nested := get(database.ConversationBranchQuery{Parent: refs["a"].Id, Limit: 100})
	if len(nested.Entries) != 4 || nested.Entries[0].Event.Ref != refs["ar"] || nested.Entries[2].Event.Ref != refs["third"] || nested.Entries[3].Event.Ref != refs["hidden"] {
		t.Fatalf("nested branch = %#v", nested.Entries)
	}
	answers := get(database.ConversationBranchQuery{Parent: refs["answer"].Id, Limit: 100})
	if len(answers.Entries) != 1 || answers.Entries[0].Event.Ref != refs["answer-child"] || *answers.Entries[0].Event.Parent != refs["answer"] {
		t.Fatalf("answer branch = %#v", answers)
	}
	// Directly visible forms also get response state, without a visible agent request.
	add("aa", "at", "approval.request", first, map[string]any{"description": "Allow historical action?"}, 13)
	add("aas", "aa", "approval.success", "", map[string]any{}, 14)
	direct := get(database.ConversationBranchQuery{Parent: refs["at"].Id, Limit: 100})
	if len(direct.Entries) != 2 || len(direct.AgentRequests) != 0 || len(direct.Interactions) != 2 || direct.Interactions[0].Response == nil || direct.Interactions[1].Response == nil {
		t.Fatalf("direct input = %#v", direct)
	}
	pending := get(database.ConversationBranchQuery{Parent: refs["bt"].Id, Limit: 100})
	if len(pending.Entries) != 2 || len(pending.Interactions) != 2 || pending.Interactions[0].Response != nil {
		t.Fatalf("direct pending prompts = %#v", pending)
	}
	empty := get(database.ConversationBranchQuery{Parent: refs["answer-child"].Id, Limit: 100})
	if len(empty.Entries) != 0 || empty.Entries == nil || empty.AgentRequests == nil || empty.Interactions == nil {
		t.Fatalf("empty branch = %#v", empty)
	}
	for _, input := range []database.ConversationBranchQuery{{Limit: 0}, {Limit: 101}, {Limit: 1, Parent: "bad"}, {Limit: 1, Cursor: "bad"}, {Limit: 1, Parent: refs["a"].Id, Cursor: page.NextCursor}} {
		if err, _ := store.SessionConversationBranchGet(ctx, session, input); !errors.Is(err, database.ErrConversationQuery) {
			t.Fatalf("accepted invalid query %#v: %v", input, err)
		}
	}
	missing := database.ConversationBranchQuery{Parent: "sev_00000000000000000000000999", Limit: 1}
	if err, _ := store.SessionConversationBranchGet(ctx, session, missing); !errors.Is(err, database.ErrConversationParent) {
		t.Fatalf("missing parent = %v", err)
	}
	other := session
	other.Id = "ses_00000000000000000000000001"
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: other, AuthorPrincipal: &person, Enabled: true}, person); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionConversationBranchGet(ctx, other, database.ConversationBranchQuery{Parent: refs["a"].Id, Limit: 1}); !errors.Is(err, database.ErrConversationParent) {
		t.Fatalf("cross-session parent = %v", err)
	}
	if err, _ := store.SessionConversationBranchGet(ctx, other, database.ConversationBranchQuery{Cursor: page.NextCursor, Limit: 1}); !errors.Is(err, database.ErrConversationQuery) {
		t.Fatalf("cross-session cursor = %v", err)
	}
	add("bis", "bi", "input.success", "", map[string]any{"result": map[string]any{}}, 16)
	add("bas", "bapproval", "approval.failure", "", map[string]any{"code": "rejected"}, 16)
	resumed := get(database.ConversationBranchQuery{Parent: refs["b"].Id, Limit: 100})
	if len(resumed.AgentRequests) != 1 || resumed.AgentRequests[0].State != "working" || len(resumed.Interactions) != 2 {
		t.Fatalf("resolved interactions did not resume working state: %#v", resumed)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err, _ := store.SessionConversationBranchGet(cancelled, session, database.ConversationBranchQuery{Limit: 1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read = %v", err)
	}
}
