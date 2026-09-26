package database_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func inputDraftFixture(t *testing.T, configuration config.DatabaseConfig) (*database.Store, model.SessionEventRef) {
	t.Helper()
	ctx := context.Background()
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	state := config.State{
		Workspaces:      []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:      []config.Principal{{Alias: "alice", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "test", Parameters: `{}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	principal := principalRef(t, ctx, store, "alice")
	person := model.Principal{Ref: principal, Enabled: true}
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &principal, Enabled: true}, principal); err != nil {
		t.Fatal(err)
	}
	var agentID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	agent := model.WorkspaceAgentRef{Workspace: workspace, Id: agentID}
	message := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &person, Payload: map[string]interface{}{"text": "hello"}}
	tool := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"}, Parent: &message.Ref, Kind: "tool.request", AuthorAgent: &agent, Payload: map[string]interface{}{}}
	input := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000002"}, Parent: &tool.Ref, Kind: "input.request", AuthorAgent: &agent, Payload: map[string]interface{}{"description": "Review"}}
	if err, _ := store.SessionEventsCreateBatch(ctx, []model.SessionEvent{message, tool, input}); err != nil {
		t.Fatal(err)
	}
	return store, input.Ref
}

func TestSessionInputDraftNestedSetAndRemove(t *testing.T) {
	ctx := context.Background()
	store, request := inputDraftFixture(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err, draft := store.SessionInputDraftGet(ctx, request); err != nil || draft != nil {
		t.Fatalf("initial SessionInputDraftGet() = (%#v, %v)", draft, err)
	}
	for _, patch := range []struct {
		path  []string
		value string
		want  string
	}{
		{[]string{"profile", "name"}, `"Ada"`, `{"profile":{"name":"Ada"}}`},
		{[]string{"profile", "notes"}, `null`, `{"profile":{"name":"Ada","notes":null}}`},
		{[]string{"profile", "x.y", `a"b`}, `true`, `{"profile":{"name":"Ada","notes":null,"x.y":{"a\"b":true}}}`},
	} {
		err, draft := store.SessionInputDraftSet(ctx, request, patch.path, json.RawMessage(patch.value))
		if err != nil || draft == nil || string(draft.Values) != patch.want || draft.Request != request || draft.UpdatedAt == "" {
			t.Fatalf("SessionInputDraftSet(%q) = (%#v, %v), want %s", patch.path, draft, err, patch.want)
		}
	}
	err, draft := store.SessionInputDraftSet(ctx, request, []string{"profile", "name"}, json.RawMessage(`"Grace"`))
	if err != nil || draft == nil || string(draft.Values) != `{"profile":{"name":"Grace","notes":null,"x.y":{"a\"b":true}}}` {
		t.Fatalf("replace existing field = (%#v, %v)", draft, err)
	}
	err, draft = store.SessionInputDraftRemove(ctx, request, []string{"profile", "notes"})
	if err != nil || draft == nil || string(draft.Values) != `{"profile":{"name":"Grace","x.y":{"a\"b":true}}}` {
		t.Fatalf("remove optional field = (%#v, %v)", draft, err)
	}
	err, draft = store.SessionInputDraftRemove(ctx, request, []string{"profile", "missing", "key"})
	if err != nil || draft == nil || string(draft.Values) != `{"profile":{"name":"Grace","x.y":{"a\"b":true}}}` {
		t.Fatalf("remove missing field = (%#v, %v)", draft, err)
	}
	err, draft = store.SessionInputDraftRemove(ctx, request, []string{"profile", "x.y"})
	if err != nil || draft == nil || string(draft.Values) != `{"profile":{"name":"Grace"}}` {
		t.Fatalf("remove nested object = (%#v, %v)", draft, err)
	}
	err, stored := store.SessionInputDraftGet(ctx, request)
	if err != nil || stored == nil || string(stored.Values) != string(draft.Values) {
		t.Fatalf("stored input draft = (%#v, %v)", stored, err)
	}
}

func TestSessionInputDraftListsAndInvalidPaths(t *testing.T) {
	ctx := context.Background()
	store, request := inputDraftFixture(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	for _, values := range []string{`["one","two"]`, `["two","one"]`, `[]`} {
		err, draft := store.SessionInputDraftSet(ctx, request, []string{"items"}, json.RawMessage(values))
		if err != nil || draft == nil || string(draft.Values) != `{"items":`+values+`}` {
			t.Fatalf("replace list with %s = (%#v, %v)", values, draft, err)
		}
	}
	for _, test := range []struct {
		path []string
		err  error
	}{
		{nil, database.ErrSessionInputDraftPath},
		{[]string{" "}, database.ErrSessionInputDraftPath},
		{[]string{"items", "0"}, database.ErrSessionInputDraftPath},
	} {
		err, draft := store.SessionInputDraftSet(ctx, request, test.path, json.RawMessage(`"changed"`))
		if !errors.Is(err, test.err) || draft != nil {
			t.Fatalf("set path %q = (%#v, %v), want %v", test.path, draft, err, test.err)
		}
	}
	if err, draft := store.SessionInputDraftSet(ctx, request, []string{"items"}, json.RawMessage(`not json`)); err == nil || draft != nil {
		t.Fatalf("invalid JSON value = (%#v, %v)", draft, err)
	}
	if err, draft := store.SessionInputDraftSet(ctx, model.SessionEventRef{Session: request.Session, Id: "sev_00000000000000000000000001"}, []string{"name"}, json.RawMessage(`"wrong kind"`)); err != nil || draft != nil {
		t.Fatalf("draft for tool.request = (%#v, %v)", draft, err)
	}
	if err, draft := store.SessionInputDraftSet(ctx, model.SessionEventRef{Session: request.Session, Id: "sev_00000000000000000000000003"}, []string{"name"}, json.RawMessage(`"missing"`)); err != nil || draft != nil {
		t.Fatalf("draft for missing request = (%#v, %v)", draft, err)
	}
	if err, draft := store.SessionInputDraftGet(ctx, request); err != nil || draft == nil || string(draft.Values) != `{"items":[]}` {
		t.Fatalf("draft after rejected patches = (%#v, %v)", draft, err)
	}
}

func TestSessionInputDraftConcurrentWritersShareNestedDocument(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "input.sqlite")}
	first, request := inputDraftFixture(t, configuration)
	err, second := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	const count = 20
	var wait sync.WaitGroup
	errors := make(chan error, count)
	for index := range count {
		wait.Add(1)
		go func() {
			defer wait.Done()
			writer := first
			if index%2 == 1 {
				writer = second
			}
			name := fmt.Sprintf("field_%02d", index)
			err, draft := writer.SessionInputDraftSet(ctx, request, []string{"answers", name}, json.RawMessage(fmt.Sprintf("%d", index)))
			if err == nil && draft == nil {
				err = fmt.Errorf("patch returned no draft")
			}
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	err, draft := second.SessionInputDraftGet(ctx, request)
	if err != nil || draft == nil {
		t.Fatalf("concurrent draft = (%#v, %v)", draft, err)
	}
	var decoded struct {
		Answers map[string]int `json:"answers"`
	}
	if err := json.Unmarshal(draft.Values, &decoded); err != nil || len(decoded.Answers) != count {
		t.Fatalf("concurrent draft values = (%s, %v)", draft.Values, err)
	}
	for index := range count {
		name := fmt.Sprintf("field_%02d", index)
		if decoded.Answers[name] != index {
			t.Fatalf("concurrent draft[%q] = %d, want %d", name, decoded.Answers[name], index)
		}
	}
}
