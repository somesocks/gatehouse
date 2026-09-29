package agent_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
	_ "github.com/dbos-inc/dbos-transact-golang/dbos/driver/sqlite"

	"gatehouse/agent"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestAcceptedCancellationFailsWhenReplyFinishesFirst(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:      []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:      []config.Principal{{Alias: "alice", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "unsupported-test-model", Parameters: `{}`, MaxTurns: 1, MaxOutputTokens: 1024, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true}},
	}
	err, ring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer ring.Close()
	err, registry := migrations.Build(configuration, state, ring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(ctx, store, registry); err != nil {
		t.Fatal(err)
	}
	err, workspace := store.WorkspaceRefGetByAlias(ctx, "engineering")
	if err != nil || workspace == nil {
		t.Fatalf("workspace = (%#v, %v)", workspace, err)
	}
	var principalID, agentID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_principals WHERE alias = 'alice'`).Scan(&principalID); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	principalRef := model.PrincipalRef{Id: principalID}
	principal := model.Principal{Ref: principalRef, Enabled: true}
	session := model.SessionRef{Workspace: *workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &principalRef, Enabled: true}, principalRef); err != nil {
		t.Fatal(err)
	}
	message := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &principal, Payload: map[string]interface{}{"text": "Hello", "agents": []string{agentID}}}
	if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
		t.Fatal(err)
	}
	err, request := store.SessionEventChildGet(ctx, message.Ref, "agent.request")
	if err != nil || request == nil {
		t.Fatalf("agent request = (%#v, %v)", request, err)
	}
	dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-cancel-race-test", SQLiteSystemDB: store.DB})
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
	replyHandle, err := dbos.RetrieveWorkflow[model.SessionEventRef](dbosContext, "session-event-reply:"+request.Ref.Id)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, err := replyHandle.GetStatus()
		if err != nil {
			t.Fatal(err)
		}
		if status.Status == dbos.WorkflowStatusError {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	status, err := replyHandle.GetStatus()
	if err != nil || status.Status != dbos.WorkflowStatusError {
		t.Fatalf("failed reply = (%#v, %v), want ERROR", status, err)
	}
	id, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	cancellation := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: id}, Parent: &request.Ref, Kind: model.SessionEventKindCancelRequest, AuthorPrincipal: &principal, Payload: map[string]interface{}{}}
	if err, _ := runtime.CancelReply(ctx, cancellation); !errors.Is(err, database.ErrSessionReplyAlreadyCompleted) {
		t.Fatalf("CancelReply() on terminal workflow = %v, want conflict", err)
	}
	handle, err := dbos.Enqueue[model.SessionEventRef](dbosContext, "gatehouse.session-tool-calls", "gatehouse.session-reply-cancel", agent.SessionReplyCancellationInput{Request: cancellation},
		dbos.WithEnqueueWorkflowID("session-reply-cancel:"+request.Ref.Id), dbos.WithEnqueueClassName("SessionEventReplyRuntime"), dbos.WithEnqueueConfigName("gatehouse"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := handle.GetResult()
	if err != nil {
		t.Fatal(err)
	}
	err, failure := store.SessionEventGet(ctx, result)
	if err != nil || failure == nil || failure.Kind != model.SessionEventKindCancelFailure || failure.Parent == nil || *failure.Parent != cancellation.Ref || failure.Payload["code"] != "already_completed" {
		t.Fatalf("cancel outcome = (%#v, %v), want cancel.failure", failure, err)
	}
	if err, success := store.SessionEventChildGet(ctx, cancellation.Ref, model.SessionEventKindCancelSuccess); err != nil || success != nil {
		t.Fatalf("unexpected cancel.success = (%#v, %v)", success, err)
	}
}
