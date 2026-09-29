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

var defaultAgentModelCompaction = config.AgentModelCompaction{
	Algorithm:    "mcmtr",
	HistoryBytes: config.DefaultAgentModelHistoryBytes,
	BufferBytes:  config.DefaultAgentModelBufferBytes,
}

func TestSessionEventReplyRuntimeCreatesOneBuiltinReply(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	aliceName := "Alice"
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:     []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:     []config.Principal{{Alias: "alice", Name: &aliceName, Enabled: true}},
		AgentProviders: []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{
			{Alias: "automatic", Revision: 1, ProviderAlias: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Automatic reply."}`, Compaction: defaultAgentModelCompaction, MaxTurns: config.DefaultAgentModelMaxTurns, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true},
			{Alias: "requested", Revision: 1, ProviderAlias: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Requested reply."}`, Compaction: defaultAgentModelCompaction, MaxTurns: config.DefaultAgentModelMaxTurns, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true},
		},
		WorkspaceAgents: []config.WorkspaceAgent{
			{WorkspaceID: "engineering", Alias: "automatic", ModelAlias: "automatic", Revision: 1, Default: true, Enabled: true},
			{WorkspaceID: "engineering", Alias: "requested", ModelAlias: "requested", Revision: 1, Enabled: true},
		},
	}
	keyringErr, keyring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if keyringErr != nil {
		t.Fatal(keyringErr)
	}
	defer keyring.Close()
	buildErr, set := migrations.Build(configuration, state, keyring)
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	if err := migrations.Run(ctx, store, set); err != nil {
		t.Fatal(err)
	}

	workspace := workspaceRef(t, ctx, store, "engineering")
	var requestedID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'requested'`).Scan(&requestedID); err != nil {
		t.Fatal(err)
	}
	alice := principalRef(t, ctx, store, "alice")
	alicePrincipal := model.Principal{Ref: alice, Name: &aliceName, Enabled: true}
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	if err, _ := store.SessionMessagesCreate(ctx, model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"},
		Kind:            "message.text",
		AuthorPrincipal: &alicePrincipal,
		Payload:         map[string]interface{}{"text": "human-only context"},
	}); err != nil {
		t.Fatal(err)
	}
	message := model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"},
		Kind:            "message.text",
		AuthorPrincipal: &alicePrincipal,
		Payload:         map[string]interface{}{"text": "hello", "agents": []string{requestedID}},
	}
	if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
		t.Fatal(err)
	}

	dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-agent-test", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	runtimeErr, runtime := agent.NewSessionEventReplyRuntime(dbosContext, store, keyring)
	if runtimeErr != nil {
		t.Fatal(runtimeErr)
	}
	if err := dbos.Launch(dbosContext); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(dbosContext, time.Second)
	if err := runtime.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Reconcile(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		err, events := store.SessionEventsGet(ctx, session)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) == 6 {
			request := events[2]
			started := events[3]
			completed := events[4]
			reply := events[5]
			if request.Kind != model.SessionEventKindAgentRequest || request.Parent == nil || *request.Parent != message.Ref || request.AuthorPrincipal == nil || request.AuthorPrincipal.Ref != alice || request.Payload["agent"] != requestedID {
				t.Fatalf("agent request event = %#v", request)
			}
			if started.Kind != model.SessionEventKindThinkingRequest || started.Parent == nil || *started.Parent != request.Ref || started.AuthorAgent == nil || started.AuthorAgent.Id != requestedID || started.Payload["turn"] != float64(0) || started.Ref.Id == "" {
				t.Fatalf("thinking start event = %#v", started)
			}
			if completed.Kind != model.SessionEventKindThinkingSuccess || completed.Parent == nil || *completed.Parent != started.Ref || completed.AuthorAgent == nil || completed.AuthorAgent.Id != requestedID || completed.Ref.Id == "" {
				t.Fatalf("thinking completion event = %#v", completed)
			}
			if reply.Kind != model.SessionEventKindAgentSuccess || reply.Parent == nil || *reply.Parent != request.Ref || reply.AuthorAgent == nil || reply.AuthorAgent.Id != requestedID || reply.Payload["text"] != "Requested reply." || reply.Ref.Id == "" {
				t.Fatalf("reply event = %#v", reply)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session events = %#v, want human-only message, targeted message, agent request, thinking lifecycle, and agent.success", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	err, tasks := store.SessionEventReplyTasksGet(ctx, 10)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("SessionEventReplyTasksGet() = (%#v, %v)", tasks, err)
	}
	for {
		err, name := store.SessionNameGet(ctx, session)
		if err != nil {
			t.Fatal(err)
		}
		if name != nil {
			if *name != "Requested reply." {
				t.Fatalf("session name = %q, want selected agent title", *name)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session name was not generated")
		}
		time.Sleep(20 * time.Millisecond)
	}
	err, nameTasks := store.SessionNameTasksGet(ctx, 10)
	if err != nil || len(nameTasks) != 0 {
		t.Fatalf("SessionNameTasksGet() = (%#v, %v)", nameTasks, err)
	}
	rows, err := store.QueryContext(ctx, `
		SELECT function_name, length(COALESCE(output, ''))
		FROM operation_outputs
		WHERE function_name = 'gatehouse.session-event-agent-persist'
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var length int
		if err := rows.Scan(&name, &length); err != nil {
			t.Fatal(err)
		}
		if length > 512 {
			t.Fatalf("DBOS operation %q stored %d bytes, want only a small event reference", name, length)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	err, finished := store.SessionEventChildGet(ctx, message.Ref, "agent.request")
	if err != nil || finished == nil {
		t.Fatalf("completed agent request = (%#v, %v)", finished, err)
	}
	cancellationID, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	cancellation := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: cancellationID}, Parent: &finished.Ref, Kind: "cancel.request", AuthorPrincipal: &alicePrincipal, Payload: map[string]interface{}{}}
	if err, _ := runtime.CancelReply(ctx, cancellation); !errors.Is(err, database.ErrSessionReplyAlreadyCompleted) {
		t.Fatalf("CancelReply() after agent.success = %v, want conflict", err)
	}
	if err, stored := store.SessionEventChildGet(ctx, finished.Ref, "cancel.request"); err != nil || stored != nil {
		t.Fatalf("cancellation request after completed reply = (%#v, %v)", stored, err)
	}
}

func TestSessionEventReplyRuntimeCancelsQueuedReply(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	aliceName := "Alice"
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		Workspaces:     []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:     []config.Principal{{Alias: "alice", Name: &aliceName, Enabled: true}},
		AgentProviders: []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{{
			Alias: "fallback", Revision: 1, ProviderAlias: "builtin", Model: agent.BuiltinModelDummyFixedReply, Parameters: `{"text":"Fallback reply."}`, Compaction: defaultAgentModelCompaction, MaxTurns: config.DefaultAgentModelMaxTurns, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true,
		}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "fallback", ModelAlias: "fallback", Revision: 1, Default: true, Enabled: true}},
	}
	keyringErr, keyring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if keyringErr != nil {
		t.Fatal(keyringErr)
	}
	defer keyring.Close()
	buildErr, set := migrations.Build(configuration, state, keyring)
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	if err := migrations.Run(ctx, store, set); err != nil {
		t.Fatal(err)
	}

	workspace := workspaceRef(t, ctx, store, "engineering")
	var agentID string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'fallback'`).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	alice := principalRef(t, ctx, store, "alice")
	alicePrincipal := model.Principal{Ref: alice, Name: &aliceName, Enabled: true}
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	message := model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"},
		Kind:            "message.text",
		AuthorPrincipal: &alicePrincipal,
		Payload:         map[string]interface{}{"text": "hello", "agents": []string{agentID}},
	}
	if err, _ := store.SessionMessagesCreate(ctx, message); err != nil {
		t.Fatal(err)
	}
	err, events := store.SessionEventsGet(ctx, session)
	if err != nil || len(events) != 2 || events[1].Kind != model.SessionEventKindAgentRequest {
		t.Fatalf("created request events = (%#v, %v)", events, err)
	}
	request := events[1]
	cancellationID, err := typed_id.New(typed_id.SessionEvent)
	if err != nil {
		t.Fatal(err)
	}
	cancellation := model.SessionEvent{
		Ref:             model.SessionEventRef{Session: session, Id: cancellationID},
		Parent:          &request.Ref,
		Kind:            model.SessionEventKindCancelRequest,
		AuthorPrincipal: &alicePrincipal,
		Payload:         map[string]interface{}{},
	}
	dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-agent-cancel-test", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	runtimeErr, runtime := agent.NewSessionEventReplyRuntime(dbosContext, store, keyring)
	if runtimeErr != nil {
		t.Fatal(runtimeErr)
	}
	if err := dbos.Launch(dbosContext); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(dbosContext, time.Second)
	if err, stored := runtime.CancelReply(ctx, cancellation); err != nil || stored.Ref != cancellation.Ref {
		t.Fatalf("CancelReply() = (%#v, %v), want %#v", stored, err, cancellation.Ref)
	}
	if err := runtime.Reconcile(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		err, events := store.SessionEventsGet(ctx, session)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) == 5 {
			failure := events[3]
			if failure.Kind != model.SessionEventKindAgentFailure || failure.Parent == nil || *failure.Parent != request.Ref || failure.Payload["code"] != "cancelled" {
				t.Fatalf("agent cancellation outcome = %#v", failure)
			}
			success := events[4]
			if success.Kind != model.SessionEventKindCancelSuccess || success.Parent == nil || *success.Parent != cancellation.Ref || success.AuthorPrincipal == nil || success.AuthorPrincipal.Ref != alice || success.AuthorPrincipal.Name == nil || *success.AuthorPrincipal.Name != aliceName {
				t.Fatalf("cancellation success = %#v", success)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session events = %#v, want message, agent request, cancellation request, agent failure, and cancellation success", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
	err, tasks := store.SessionEventReplyTasksGet(ctx, 10)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("SessionEventReplyTasksGet() = (%#v, %v)", tasks, err)
	}
}

func workspaceRef(t *testing.T, ctx context.Context, store *database.Store, alias string) model.WorkspaceRef {
	t.Helper()
	err, workspace := store.WorkspaceRefGetByAlias(ctx, alias)
	if err != nil {
		t.Fatal(err)
	}
	if workspace == nil {
		t.Fatalf("workspace alias %q was not found", alias)
	}
	return *workspace
}

func principalRef(t *testing.T, ctx context.Context, store *database.Store, alias string) model.PrincipalRef {
	t.Helper()
	var id string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_principals WHERE alias = ?`, alias).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return model.PrincipalRef{Id: id}
}
