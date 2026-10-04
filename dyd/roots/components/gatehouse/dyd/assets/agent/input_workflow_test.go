package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
)

func inputDeliveryFixture(t *testing.T, configuration config.DatabaseConfig) (*database.Store, model.SessionEventRef, model.Principal) {
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
	return store, request.Ref, principal
}

func inputDeliveryTool(t *testing.T, store *database.Store, request model.SessionEventRef) model.SessionEventRef {
	t.Helper()
	err, input := store.SessionEventGet(context.Background(), request)
	if err != nil || input == nil || input.Parent == nil {
		t.Fatalf("input request tool: %#v (%v)", input, err)
	}
	return *input.Parent
}

func inputDeliveryWait(ctx dbos.Context, request model.SessionEventRef) (model.SessionEventRef, error) {
	if os.Getenv("GATEHOUSE_TEST_INPUT_DELIVERY_CRASH") != "" {
		// Leave the durable message unconsumed until the process crashes.
		select {}
	}
	return dbos.Recv[model.SessionEventRef](ctx, "input:"+request.Id, time.Minute)
}

func inputDeliveryWaitBoth(ctx dbos.Context, requests [2]model.SessionEventRef) ([2]model.SessionEventRef, error) {
	var responses [2]model.SessionEventRef
	for index, request := range requests {
		response, err := dbos.Recv[model.SessionEventRef](ctx, "input:"+request.Id, time.Minute)
		if err != nil {
			return responses, err
		}
		responses[index] = response
	}
	return responses, nil
}

func inputDeliveryRuntime(t *testing.T, store *database.Store) (dbos.Context, *agent.SessionEventReplyRuntime) {
	t.Helper()
	ctx := context.Background()
	err, keyring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(keyring.Close)
	dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-input-delivery-test", ApplicationVersion: "input-delivery-v1", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	err, runtime := agent.NewSessionEventReplyRuntime(dbosContext, store, keyring)
	if err != nil {
		t.Fatal(err)
	}
	dbos.RegisterWorkflow(dbosContext, inputDeliveryWait, dbos.WithWorkflowName("test.session-input-delivery"))
	dbos.RegisterWorkflow(dbosContext, inputDeliveryWaitBoth, dbos.WithWorkflowName("test.session-input-delivery-both"))
	if err := dbos.Launch(dbosContext); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbos.Shutdown(dbosContext, time.Second) })
	return dbosContext, runtime
}

// A failed send must roll back the event, its activity, and the draft deletion.
// A successful submission reaches DBOS without a reconciliation pass.
func TestInputResponseCommitsWithDBOSMessage(t *testing.T) {
	ctx := context.Background()
	store, request, principal := inputDeliveryFixture(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	dbosContext, runtime := inputDeliveryRuntime(t, store)
	if err, _ := store.SessionInputDraftSet(ctx, request, []string{"name"}, json.RawMessage(`"Ada"`)); err != nil {
		t.Fatal(err)
	}
	response := model.SessionEvent{Ref: model.SessionEventRef{Session: request.Session, Id: "sev_00000000000000000000000003"}, Parent: &request, Kind: "input.success", AuthorPrincipal: &principal}
	if err, _ := runtime.RespondToInput(ctx, response); err == nil {
		t.Fatal("accepted a response without a destination workflow")
	}
	if err, event := store.SessionEventGet(ctx, response.Ref); err != nil || event != nil {
		t.Fatalf("failed send persisted a response: %#v (%v)", event, err)
	}
	if err, draft := store.SessionInputDraftGet(ctx, request); err != nil || draft == nil || string(draft.Values) != `{"name":"Ada"}` {
		t.Fatalf("failed send changed the draft: %#v (%v)", draft, err)
	}
	var activity int
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events WHERE resource_session_event = ?`, response.Ref.Id).Scan(&activity); err != nil || activity != 0 {
		t.Fatalf("failed send persisted response activity: %d (%v)", activity, err)
	}
	tool := inputDeliveryTool(t, store, request)
	handle, err := dbos.RunWorkflow(dbosContext, inputDeliveryWait, request, dbos.WithWorkflowID("session-tool-call:"+tool.Id))
	if err != nil {
		t.Fatal(err)
	}
	if err, _ := runtime.RespondToInput(ctx, response); err != nil {
		t.Fatal(err)
	}
	got, err := handle.GetResult()
	if err != nil || got != response.Ref {
		t.Fatalf("delivered response = (%#v, %v), want %#v", got, err, response.Ref)
	}
}

func TestInputResponsesUseDistinctTopicsOnOneTool(t *testing.T) {
	ctx := context.Background()
	store, first, principal := inputDeliveryFixture(t, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	err, next := store.SessionEventGet(ctx, first)
	if err != nil || next == nil {
		t.Fatalf("first form: %#v (%v)", next, err)
	}
	next.Ref.Id = "sev_00000000000000000000000005"
	if err, _ := store.SessionEventsCreateBatch(ctx, []model.SessionEvent{*next}); err != nil {
		t.Fatal(err)
	}
	second := next.Ref
	const firstMarker = "FIRST_FORM_RESPONSE_MUST_STAY_IN_GATEHOUSE_b48ae1"
	const secondMarker = "SECOND_FORM_RESPONSE_MUST_STAY_IN_GATEHOUSE_f2d723"
	for _, input := range []struct {
		request model.SessionEventRef
		value   string
	}{{first, firstMarker}, {second, secondMarker}} {
		value, _ := json.Marshal(input.value)
		if err, _ := store.SessionInputDraftSet(ctx, input.request, []string{"name"}, value); err != nil {
			t.Fatal(err)
		}
	}
	dbosContext, runtime := inputDeliveryRuntime(t, store)
	tool := inputDeliveryTool(t, store, first)
	handle, err := dbos.RunWorkflow(dbosContext, inputDeliveryWaitBoth, [2]model.SessionEventRef{first, second}, dbos.WithWorkflowID("session-tool-call:"+tool.Id))
	if err != nil {
		t.Fatal(err)
	}
	firstResponse := model.SessionEvent{Ref: model.SessionEventRef{Session: first.Session, Id: "sev_00000000000000000000000003"}, Parent: &first, Kind: "input.success", AuthorPrincipal: &principal}
	secondResponse := model.SessionEvent{Ref: model.SessionEventRef{Session: second.Session, Id: "sev_00000000000000000000000006"}, Parent: &second, Kind: "input.success", AuthorPrincipal: &principal}
	// Queue the later form first. It must not satisfy the earlier form's wait.
	if err, _ := runtime.RespondToInput(ctx, secondResponse); err != nil {
		t.Fatal(err)
	}
	if err, _ := runtime.RespondToInput(ctx, firstResponse); err != nil {
		t.Fatal(err)
	}
	got, err := handle.GetResult()
	if err != nil || got != [2]model.SessionEventRef{firstResponse.Ref, secondResponse.Ref} {
		t.Fatalf("form responses crossed topics: %#v (%v)", got, err)
	}
	assertDBOSDoesNotContain(t, store, firstMarker)
	assertDBOSDoesNotContain(t, store, secondMarker)
}

func TestInputResponseDeliveryCrashProcess(t *testing.T) {
	path := os.Getenv("GATEHOUSE_TEST_INPUT_DELIVERY_PATH")
	if path == "" {
		t.Skip("subprocess helper")
	}
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	dbosContext, runtime := inputDeliveryRuntime(t, store)
	var workspace string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_workspaces WHERE alias = 'engineering'`).Scan(&workspace); err != nil {
		t.Fatal(err)
	}
	request := model.SessionEventRef{Session: model.SessionRef{Workspace: model.WorkspaceRef{Id: workspace}, Id: "ses_00000000000000000000000000"}, Id: "sev_00000000000000000000000002"}
	principal := model.Principal{Ref: principalRef(t, ctx, store, "alice"), Enabled: true}
	tool := inputDeliveryTool(t, store, request)
	if _, err := dbos.RunWorkflow(dbosContext, inputDeliveryWait, request, dbos.WithWorkflowID("session-tool-call:"+tool.Id)); err != nil {
		t.Fatal(err)
	}
	response := model.SessionEvent{Ref: model.SessionEventRef{Session: request.Session, Id: "sev_00000000000000000000000003"}, Parent: &request, Kind: "input.success", AuthorPrincipal: &principal}
	if os.Getenv("GATEHOUSE_TEST_INPUT_DELIVERY_CRASH") == "before-commit" {
		tx, err := store.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err, _ := store.SessionInputResponseCreateInTransaction(ctx, tx, response); err != nil {
			t.Fatal(err)
		}
		if err := dbos.Send(dbosContext, "session-tool-call:"+tool.Id, response.Ref, "input:"+request.Id, dbos.WithSendTransaction(tx)); err != nil {
			t.Fatal(err)
		}
	} else if err, _ := runtime.RespondToInput(ctx, response); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func TestInputResponseDeliveryAcrossCrash(t *testing.T) {
	for _, boundary := range []string{"before-commit", "after-commit"} {
		t.Run(boundary, func(t *testing.T) {
			ctx := context.Background()
			configuration := config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "input.sqlite")}
			store, request, principal := inputDeliveryFixture(t, configuration)
			tool := inputDeliveryTool(t, store, request)
			const marker = "DBOS_INPUT_DELIVERY_PAYLOAD_MUST_STAY_IN_GATEHOUSE_45a3bc"
			answer := marker + strings.Repeat("x", 256<<10)
			value, err := json.Marshal(answer)
			if err != nil {
				t.Fatal(err)
			}
			want := `{"name":` + string(value) + `}`
			if err, _ := store.SessionInputDraftSet(ctx, request, []string{"name"}, value); err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			process := exec.Command(os.Args[0], "-test.run=^TestInputResponseDeliveryCrashProcess$", "-test.timeout=30s")
			process.Env = append(os.Environ(), "GATEHOUSE_TEST_INPUT_DELIVERY_PATH="+configuration.Path, "GATEHOUSE_TEST_INPUT_DELIVERY_CRASH="+boundary)
			if output, err := process.CombinedOutput(); err != nil {
				t.Fatalf("delivery crash process: %v\n%s", err, output)
			}
			err, store = database.Open(ctx, configuration)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { store.Close() })
			response := model.SessionEvent{Ref: model.SessionEventRef{Session: request.Session, Id: "sev_00000000000000000000000003"}, Parent: &request, Kind: "input.success", AuthorPrincipal: &principal}
			err, stored := store.SessionEventGet(ctx, response.Ref)
			if err != nil {
				t.Fatal(err)
			}
			if boundary == "before-commit" && stored != nil || boundary == "after-commit" && stored == nil {
				t.Fatalf("response at %s: %#v", boundary, stored)
			}
			if err, draft := store.SessionInputDraftGet(ctx, request); err != nil || boundary == "before-commit" && (draft == nil || string(draft.Values) != want) || boundary == "after-commit" && draft != nil {
				t.Fatalf("draft at %s: %#v (%v)", boundary, draft, err)
			}
			// Inspect pending messages as well as completed checkpoints.
			assertDBOSDoesNotContain(t, store, marker)
			dbosContext, runtime := inputDeliveryRuntime(t, store)
			if boundary == "before-commit" {
				response.Ref.Id = "sev_00000000000000000000000004"
				if err, _ := runtime.RespondToInput(ctx, response); err != nil {
					t.Fatal(err)
				}
			}
			handle, err := dbos.RetrieveWorkflow[model.SessionEventRef](dbosContext, "session-tool-call:"+tool.Id)
			if err != nil {
				t.Fatal(err)
			}
			got, err := handle.GetResult()
			if err != nil || got != response.Ref {
				t.Fatalf("response after recovery: %#v (%v), want %#v", got, err, response.Ref)
			}
			if err, result := store.SessionInputResponseResultGet(ctx, got); err != nil || result != want {
				t.Fatalf("answer after recovery: got %d bytes, want %d (%v)", len(result), len(want), err)
			}
			assertDBOSDoesNotContain(t, store, marker)
		})
	}
}
