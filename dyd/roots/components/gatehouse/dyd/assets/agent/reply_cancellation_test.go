package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	"gatehouse/agent"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestReplyCancellationMarksBlockedInputWorkflowsCancelled(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"output":[{"type":"function_call","call_id":"call-1","name":"lisp","arguments":"{\"code\":\"(input/ask (input/form \\\"Review\\\" (input/form/text \\\"name\\\" \\\"Name\\\")))\",\"reason\":\"review\"}"},{"type":"function_call","call_id":"call-2","name":"lisp","arguments":"{\"code\":\"(+ 1 2)\",\"reason\":\"arithmetic\"}"}],"usage":{"input_tokens":20,"output_tokens":10,"total_tokens":30}}`))
	}))
	defer server.Close()
	t.Setenv("GATEHOUSE_TEST_CANCEL_KEYCHAIN", "test passphrase")
	t.Setenv("GATEHOUSE_TEST_CANCEL_API_KEY", "test-key")
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	keychains := []config.Keychain{{ID: "cancel-test", Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_CANCEL_KEYCHAIN"}}}
	keychainID := "cancel-test"
	baseURL := server.URL + "/v1"
	state := config.State{
		Keychains:       keychains,
		Workspaces:      []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:      []config.Principal{{Alias: "alice", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "test", Revision: 1, Protocol: "openai-responses", BaseURL: &baseURL, Keychain: &keychainID, Sources: []config.AgentProviderAPIKeySource{"env:GATEHOUSE_TEST_CANCEL_API_KEY"}, Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "test", Model: "test-model", Parameters: `{}`, Compaction: defaultAgentModelCompaction, MaxTurns: 4, MaxOutputTokens: 1024, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true}},
	}
	err, ring := keychain.NewKeyring(store, keychains, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer ring.Close()
	err, migrationsSet := migrations.Build(configuration, state, ring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(ctx, store, migrationsSet); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	principalRef := principalRef(t, ctx, store, "alice")
	principal := model.Principal{Ref: principalRef, Enabled: true}
	var agentID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &principalRef, Enabled: true}, principalRef); err != nil {
		t.Fatal(err)
	}
	message := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &principal, Payload: map[string]interface{}{"text": "Review", "agents": []string{agentID}}}
	if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
		t.Fatal(err)
	}
	err, request := store.SessionEventChildGet(ctx, message.Ref, "agent.request")
	if err != nil || request == nil {
		t.Fatalf("agent request = (%#v, %v)", request, err)
	}
	dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-cancel-input-test", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	err, runtime := agent.NewSessionEventReplyRuntime(dbosContext, store, ring)
	if err != nil {
		t.Fatal(err)
	}
	if err := dbos.Launch(dbosContext); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(dbosContext, 5*time.Second)
	if err := runtime.Reconcile(); err != nil {
		t.Fatal(err)
	}
	var input model.SessionEventRef
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		err, events := store.SessionEventsGet(ctx, session)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.Kind == "input.request" {
				input = event.Ref
				break
			}
		}
		if input.Id != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if input.Id == "" {
		t.Fatal("tool did not open its input form")
	}
	id, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	cancellation := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: id}, Parent: &request.Ref, Kind: "cancel.request", AuthorPrincipal: &principal, Payload: map[string]interface{}{}}
	if err, stored := runtime.CancelReply(ctx, cancellation); err != nil || stored.Ref != cancellation.Ref {
		t.Fatalf("CancelReply() = (%#v, %v), want %#v", stored, err, cancellation.Ref)
	}
	if err, stored := runtime.CancelReply(ctx, cancellation); err != nil || stored.Ref != cancellation.Ref {
		t.Fatalf("repeated CancelReply() = (%#v, %v)", stored, err)
	}
	var success *model.SessionEvent
	for time.Now().Before(deadline.Add(10 * time.Second)) {
		err, success = store.SessionEventChildGet(ctx, cancellation.Ref, "cancel.success")
		if err != nil {
			t.Fatal(err)
		}
		if success != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if success == nil {
		t.Fatal("DBOS cancellation did not produce cancel.success")
	}
	requestedAt, err := typed_id.Timestamp(typed_id.SessionEvent, cancellation.Ref.Id)
	if err != nil {
		t.Fatal(err)
	}
	completedAt, err := typed_id.Timestamp(typed_id.SessionEvent, success.Ref.Id)
	if err != nil {
		t.Fatal(err)
	}
	if completedAt.Sub(requestedAt) < time.Second {
		t.Fatalf("cancel.success followed cancel.request after %v, want at least a one-second grace", completedAt.Sub(requestedAt))
	}
	err, inputRequest := store.SessionEventGet(ctx, input)
	if err != nil || inputRequest == nil || inputRequest.Parent == nil {
		t.Fatalf("input request = (%#v, %v)", inputRequest, err)
	}
	for _, workflowID := range []string{
		"session-event-reply:" + request.Ref.Id,
		"session-tool-call:" + inputRequest.Parent.Id,
		"session-input:" + input.Id,
	} {
		var status string
		if err := store.QueryRowContext(ctx, `SELECT status FROM workflow_status WHERE workflow_uuid = ?`, workflowID).Scan(&status); err != nil || status != "CANCELLED" {
			t.Fatalf("workflow %q = (%q, %v), want CANCELLED", workflowID, status, err)
		}
	}
	if err, resolved := store.SessionInputResolvedGet(ctx, input); err != nil || resolved {
		t.Fatalf("form after cancellation = (%v, %v), want unresolved during this phase", resolved, err)
	}
	if err, _ := store.SessionInputDraftSet(ctx, input, []string{"name"}, json.RawMessage(`"Ada"`)); err != nil {
		t.Fatalf("form draft remains editable during this phase: %v", err)
	}
	responseID, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	response := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: responseID}, Parent: &input, Kind: "input.success", AuthorPrincipal: &principal, Payload: map[string]interface{}{}}
	if err, _ := store.SessionInputResponseCreate(ctx, response); err != nil {
		t.Fatalf("form remains submitable during this phase: %v", err)
	}
	if err, stored := runtime.CancelReply(ctx, cancellation); err != nil || stored.Ref != cancellation.Ref {
		t.Fatalf("CancelReply() after completion = (%#v, %v)", stored, err)
	}
	err, events := store.SessionEventsGet(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	cancellations := 0
	for _, event := range events {
		if event.Kind == "agent.reply" {
			t.Fatal("cancelled reply produced an agent reply")
		}
		if event.Kind == "cancel.success" {
			cancellations++
		}
	}
	if cancellations != 1 {
		t.Fatalf("cancel.success count = %d, want 1", cancellations)
	}
}
