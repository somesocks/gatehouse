package agent_test

import (
	"context"
	"path/filepath"
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

func TestReplyCancellationStopsQueuedReplyAcrossRestart(t *testing.T) {
	t.Setenv("DBOS__APPVERSION", "")
	t.Setenv("DBOS__VMID", "gatehouse-cancellation-recovery-test")
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "gatehouse.sqlite")}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:      []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:      []config.Principal{{Alias: "alice", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Hello"}`, Compaction: defaultAgentModelCompaction, MaxTurns: 1, MaxOutputTokens: 1024, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true}},
	}
	err, ring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
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
	message := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &principal, Payload: map[string]interface{}{"text": "Hello", "agents": []string{agentID}}}
	if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
		t.Fatal(err)
	}
	err, request := store.SessionEventChildGet(ctx, message.Ref, "agent.request")
	if err != nil || request == nil {
		t.Fatalf("agent request = (%#v, %v)", request, err)
	}
	started := make(chan struct{}, 1)
	blocker := func(workflowContext dbos.Context, _ struct{}) (bool, error) {
		started <- struct{}{}
		return dbos.Recv[bool](workflowContext, "continue", time.Hour)
	}
	const blockerID = "test.reply-cancellation-queue-blocker"
	newRuntime := func(store *database.Store, ring *keychain.Keyring) (dbos.Context, *agent.SessionEventReplyRuntime) {
		t.Helper()
		dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-reply-cancel-recovery", ApplicationVersion: "gatehouse-reply-cancel-recovery-v1", SQLiteSystemDB: store.DB})
		if err != nil {
			t.Fatal(err)
		}
		err, runtime := agent.NewSessionEventReplyRuntime(dbosContext, store, ring)
		if err != nil {
			t.Fatal(err)
		}
		dbos.RegisterWorkflow(dbosContext, blocker, dbos.WithWorkflowName("test.reply-cancellation-queue-blocker"))
		return dbosContext, runtime
	}
	dbosContext, runtime := newRuntime(store, ring)
	if err := dbos.Launch(dbosContext); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(dbosContext, 5*time.Second)
	queue, err := dbos.RetrieveQueue(dbosContext, "gatehouse.session-event-replies")
	if err != nil {
		t.Fatal(err)
	}
	partition := workspace.Id + "/" + session.Id + "/" + request.Ref.Id
	if _, err := dbos.RunWorkflow(dbosContext, blocker, struct{}{}, dbos.WithWorkflowID(blockerID), dbos.WithQueue(queue), dbos.WithQueuePartitionKey(partition)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("queue blocker did not start")
	}
	id, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	cancellation := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: id}, Parent: &request.Ref, Kind: model.SessionEventKindCancelRequest, AuthorPrincipal: &principal, Payload: map[string]interface{}{}}
	if err, _ := runtime.CancelReply(ctx, cancellation); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		err, success := store.SessionEventChildGet(ctx, cancellation.Ref, model.SessionEventKindCancelSuccess)
		if err != nil {
			t.Fatal(err)
		}
		if success != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err, success := store.SessionEventChildGet(ctx, cancellation.Ref, model.SessionEventKindCancelSuccess); err != nil || success == nil {
		t.Fatalf("queued reply cancellation = (%#v, %v), want success while queue is blocked", success, err)
	}
	var status string
	if err := store.QueryRowContext(ctx, `SELECT status FROM workflow_status WHERE workflow_uuid = ?`, "session-event-reply:"+request.Ref.Id).Scan(&status); err != nil || status != "CANCELLED" {
		t.Fatalf("queued reply workflow = (%q, %v), want CANCELLED", status, err)
	}
	if err := dbos.Shutdown(dbosContext, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	ring.Close()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	err, store = database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err, ring = keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer ring.Close()
	dbosContext, runtime = newRuntime(store, ring)
	if err := dbos.Launch(dbosContext); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(dbosContext, 5*time.Second)
	if err, stored := runtime.CancelReply(ctx, cancellation); err != nil || stored.Ref != cancellation.Ref {
		t.Fatalf("cancel request after restart = (%#v, %v)", stored, err)
	}
	if err := runtime.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRowContext(ctx, `SELECT status FROM workflow_status WHERE workflow_uuid = ?`, "session-event-reply:"+request.Ref.Id).Scan(&status); err != nil || status != "CANCELLED" {
		t.Fatalf("reply workflow after restart = (%q, %v), want CANCELLED", status, err)
	}
	err, events := store.SessionEventsGet(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	completions := 0
	for _, event := range events {
		if event.Kind == model.SessionEventKindCancelSuccess {
			completions++
		}
		if event.Kind == model.SessionEventKindAgentSuccess {
			t.Fatal("cancelled queued reply ran after restart")
		}
	}
	if completions != 1 {
		t.Fatalf("cancel.success count after restart = %d, want 1", completions)
	}
	if err := dbos.Send(dbosContext, blockerID, true, "continue"); err != nil {
		t.Fatal(err)
	}
}
