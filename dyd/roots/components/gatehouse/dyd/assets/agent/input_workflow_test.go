package agent_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
	_ "github.com/dbos-inc/dbos-transact-golang/dbos/driver/sqlite"

	"gatehouse/agent"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/inputform"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"gatehouse/typed_id"
)

// Reconciliation must deliver a committed input response to a workflow even
// when the HTTP caller (and therefore its original process) is gone.
func TestInputResponseReconcileDeliversToWaitingWorkflow(t *testing.T) {
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
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "test", Parameters: `{}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true}},
	}
	err, keyring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer keyring.Close()
	err, registry := migrations.Build(configuration, state, keyring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(ctx, store, registry); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	principalRef := principalRef(t, ctx, store, "alice")
	principal := model.Principal{Ref: principalRef, Enabled: true}
	var agentID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	agentRef := model.WorkspaceAgentRef{Workspace: workspace, Id: agentID}
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &principalRef, Enabled: true}, principalRef); err != nil {
		t.Fatal(err)
	}
	message := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &principal, Payload: map[string]interface{}{"text": "Review"}}
	tool := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"}, Parent: &message.Ref, Kind: "tool.request", AuthorAgent: &agentRef, Payload: map[string]interface{}{}}
	form := inputform.Form{Version: inputform.Version, Type: "form", Title: "Review", Fields: []inputform.Field{{ID: "name", Label: "Name", Type: "text"}}}
	request := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000002"}, Parent: &tool.Ref, Kind: "input.request", AuthorAgent: &agentRef, Payload: map[string]interface{}{"description": form.Title, "form": form}}
	if err, _ := store.SessionEventsCreateBatch(ctx, []model.SessionEvent{message, tool, request}); err != nil {
		t.Fatal(err)
	}
	dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-input-delivery-test", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	err, runtime := agent.NewSessionEventReplyRuntime(dbosContext, store, keyring)
	if err != nil {
		t.Fatal(err)
	}
	wait := func(workflowContext dbos.Context, _ struct{}) (model.SessionEventRef, error) {
		return dbos.Recv[model.SessionEventRef](workflowContext, "response", time.Hour)
	}
	dbos.RegisterWorkflow(dbosContext, wait, dbos.WithWorkflowName("test.session-input-delivery"))
	if err := dbos.Launch(dbosContext); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(dbosContext, time.Second)
	handle, err := dbos.RunWorkflow(dbosContext, wait, struct{}{}, dbos.WithWorkflowID("session-input:"+request.Ref.Id))
	if err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionInputDraftSet(ctx, request.Ref, []string{"name"}, json.RawMessage(`"Ada"`)); err != nil {
		t.Fatal(err)
	}
	responseID, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	response := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: responseID}, Parent: &request.Ref, Kind: "input.success", AuthorPrincipal: &principal}
	if err, _ := store.SessionInputResponseCreate(ctx, response); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Reconcile(); err != nil {
		t.Fatal(err)
	}
	got, err := handle.GetResult()
	if err != nil || got != response.Ref {
		t.Fatalf("delivered response = (%#v, %v), want %#v", got, err, response.Ref)
	}
	if err := runtime.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if err, pending := store.SessionInputResponseTasksGet(ctx, 10); err != nil || len(pending) != 0 {
		t.Fatalf("pending input delivery = (%#v, %v)", pending, err)
	}
}
