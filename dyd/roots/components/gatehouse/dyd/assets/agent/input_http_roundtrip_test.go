package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
	_ "github.com/dbos-inc/dbos-transact-golang/dbos/driver/sqlite"

	"gatehouse/agent"
	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/httpservice"
	"gatehouse/inputform"
	"gatehouse/keychain"
	"gatehouse/lisp"
	"gatehouse/migrations"
	"gatehouse/model"
	"gatehouse/typed_id"
)

// Exercise the actual Lisp wrapper, HTTP form API, stored response and DBOS
// delivery together. The browser's field renderer is covered by web tests.
func TestInputAskCompletesThroughFormHTTPAPI(t *testing.T) {
	ctx := context.Background()
	databaseConfig := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, databaseConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	t.Setenv("GATEHOUSE_TEST_INPUT_KEYCHAIN", "test passphrase")
	keychains := []config.Keychain{{ID: "input-test", Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_INPUT_KEYCHAIN"}}}
	state := config.State{
		Keychains:       keychains,
		Workspaces:      []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals:      []config.Principal{{Alias: "alice", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "test", Parameters: `{}`, MaxTurns: 1, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true}},
	}
	err, ring := keychain.NewKeyring(store, keychains, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer ring.Close()
	err, registry := migrations.Build(databaseConfig, state, ring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(ctx, store, registry); err != nil {
		t.Fatal(err)
	}
	var workspaceID, principalID, agentID string
	for _, query := range []struct {
		sql    string
		output *string
	}{
		{`SELECT id FROM gatehouse_workspaces WHERE alias = 'engineering'`, &workspaceID},
		{`SELECT id FROM gatehouse_principals WHERE alias = 'alice'`, &principalID},
		{`SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`, &agentID},
	} {
		if err := store.QueryRowContext(ctx, query.sql).Scan(query.output); err != nil {
			t.Fatal(err)
		}
	}
	identityID, err := typed_id.New(typed_id.Identity)
	if err != nil {
		t.Fatal(err)
	}
	principalRef := model.PrincipalRef{Id: principalID}
	if err := store.IdentitiesUpsertRevisions(ctx, []model.Identity{{
		Id: identityID, Key: "gatehouse:alice", Principal: principalRef, Revision: 1,
		Verifiers: []interface{}{"gh-ver:invalid"}, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: workspaceID}, Id: "ses_00000000000000000000000000"}
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &principalRef, Enabled: true}, principalRef); err != nil {
		t.Fatal(err)
	}
	principal := model.Principal{Ref: principalRef, Enabled: true}
	agentRef := model.WorkspaceAgentRef{Workspace: session.Workspace, Id: agentID}
	message := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &principal, Payload: map[string]interface{}{"text": "Review"}}
	tool := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"}, Parent: &message.Ref, Kind: "tool.request", AuthorAgent: &agentRef, Payload: map[string]interface{}{"name": "lisp"}}
	if err, _ := store.SessionEventsCreateBatch(ctx, []model.SessionEvent{message, tool}); err != nil {
		t.Fatal(err)
	}
	dbosContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-input-http-test", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	err, runtime := agent.NewSessionEventReplyRuntime(dbosContext, store, ring)
	if err != nil {
		t.Fatal(err)
	}
	wait := func(waitContext dbos.Context, _ struct{}) (model.SessionEventRef, error) {
		return dbos.Recv[model.SessionEventRef](waitContext, "response", time.Hour)
	}
	dbos.RegisterWorkflow(dbosContext, wait, dbos.WithWorkflowName("test.input-http-wait"))
	workflow := func(workflowContext dbos.Context, _ struct{}) (string, error) {
		modules := []lisp.HostModule{agent.NewInputModule(func(form inputform.Form) (error, string) {
			id, err := typed_id.New(typed_id.SessionEvent)
			if err != nil {
				return err, ""
			}
			input := model.SessionEvent{
				Ref: model.SessionEventRef{Session: session, Id: id}, Parent: &tool.Ref,
				Kind: "input.request", AuthorAgent: &agentRef,
				Payload: map[string]interface{}{"description": form.Title, "form": form},
			}
			if err, _ := store.SessionEventsCreateBatch(workflowContext, []model.SessionEvent{input}); err != nil {
				return err, ""
			}
			handle, err := dbos.RunWorkflow(workflowContext, wait, struct{}{}, dbos.WithWorkflowID("session-input:"+id))
			if err != nil {
				return err, ""
			}
			response, err := handle.GetResult()
			if err != nil {
				return err, ""
			}
			return store.SessionInputResponseResultGet(workflowContext, response)
		})}
		err, result := lisp.Evaluate(`(import (json @native:json/v1) (import (input @native:gatehouse/input/v1) (let ((input/ask (fn (form) (json/decode (input/ask-json form))))) (json/string/value (json/object/get (input/ask (input/form "Review" (input/form/text "name" "Name"))) "name")))))`, lisp.EvalOptions{
			Context: workflowContext, HostModules: modules,
		})
		if err != nil {
			return "", err
		}
		return result.String(), nil
	}
	dbos.RegisterWorkflow(dbosContext, workflow, dbos.WithWorkflowName("test.input-http-roundtrip"))
	if err := dbos.Launch(dbosContext); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(dbosContext, time.Second)
	handle, err := dbos.RunWorkflow(dbosContext, workflow, struct{}{}, dbos.WithWorkflowID("test.input-http-roundtrip"))
	if err != nil {
		t.Fatal(err)
	}
	var request model.SessionEventRef
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		err, event := store.SessionEventChildGet(ctx, tool.Ref, "input.request")
		if err != nil {
			t.Fatal(err)
		}
		if event != nil {
			request = event.Ref
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if request.Id == "" {
		t.Fatal("Lisp input/ask did not persist an input.request")
	}
	err, tokens := auth.Prepare(ctx, store, ring, "input-test")
	if err != nil {
		t.Fatal(err)
	}
	err, login := tokens.Mint(ctx, auth.Claims{Principal: principal, Identity: identityID})
	if err != nil {
		t.Fatal(err)
	}
	httpHandler := httpservice.HandlerWithReplyDispatcher(config.HTTPService{API: true, PublicBaseURL: "http://localhost:4283"}, store, runtime, tokens)
	call := func(method, path, credential, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+credential)
		httpHandler.ServeHTTP(recorder, request)
		return recorder
	}
	open := fmt.Sprintf("/api/v1/workspaces/%s/sessions/%s/inputs/%s/open?redirect=false", workspaceID, session.Id, request.Id)
	opened := call(http.MethodGet, open, login, "")
	if opened.Code != http.StatusOK {
		t.Fatalf("open Lisp input = %d %s", opened.Code, opened.Body.String())
	}
	var destination struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(opened.Body.Bytes(), &destination); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(destination.URL)
	if err != nil {
		t.Fatal(err)
	}
	capability := url.Values{}
	if values, err := url.ParseQuery(parsed.RawFragment); err == nil {
		capability = values
	} else {
		t.Fatal(err)
	}
	if capability.Get("capability") == "" {
		t.Fatal("open URL has no input capability")
	}
	if saved := call(http.MethodPatch, "/api/v1/input/draft", capability.Get("capability"), `{"op":"set","path":["name"],"value":"Ada"}`); saved.Code != http.StatusNoContent {
		t.Fatalf("save input field = %d %s", saved.Code, saved.Body.String())
	}
	if submitted := call(http.MethodPost, "/api/v1/input/submit", capability.Get("capability"), ""); submitted.Code != http.StatusAccepted {
		t.Fatalf("submit input = %d %s", submitted.Code, submitted.Body.String())
	}
	result, err := handle.GetResult()
	if err != nil || result != `"Ada"` {
		t.Fatalf("Lisp resumed with json/object result = (%s, %v)", result, err)
	}
}
