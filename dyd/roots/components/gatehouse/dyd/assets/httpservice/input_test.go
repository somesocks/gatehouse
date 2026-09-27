package httpservice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/configschema"
	"gatehouse/database"
	"gatehouse/inputform"
	"gatehouse/model"
	"gatehouse/typed_id"
)

type inputTestDispatcher struct{ calls int }

func (dispatcher *inputTestDispatcher) Reconcile() error {
	dispatcher.calls++
	return nil
}

type inputTestFixture struct {
	store      *database.Store
	tokens     *auth.BearerTokens
	handler    http.Handler
	dispatcher *inputTestDispatcher
	input      model.SessionEventRef
	login      string
}

func newInputTestFixture(t *testing.T) inputTestFixture {
	t.Helper()
	ctx := context.Background()
	tokens, store, workspaces := testBearerTokens(t)
	alice, identity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, login := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	session := model.SessionRef{Workspace: workspaces["engineering"], Id: "ses_00000000000000000000000000"}
	var agentID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_workspace_agents WHERE alias = 'assistant'`).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Id: agentID}
	message := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}, Kind: "message.text", AuthorPrincipal: &model.Principal{Ref: alice, Enabled: true}, Payload: map[string]interface{}{"text": "hello"}}
	tool := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"}, Parent: &message.Ref, Kind: "tool.request", AuthorAgent: &agent, Payload: map[string]interface{}{}}
	minimum := int64(9007199254740993)
	form := inputform.Form{Version: inputform.Version, Type: "form", Title: "Review", Fields: []inputform.Field{
		{ID: "name", Label: "Name", Type: "text"},
		{ID: "profile", Label: "Profile", Type: "object", Optional: true, Fields: []inputform.Field{
			{ID: "city", Label: "City", Type: "text"},
			{ID: "entries", Label: "Entries", Type: "list", Optional: true, Item: &inputform.Field{Type: "object", Fields: []inputform.Field{
				{ID: "count", Label: "Count", Type: "number", Min: &minimum},
			}}},
		}},
		{ID: "agreed", Label: "Agreed", Type: "boolean", Optional: true},
	}}
	input := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000002"}, Parent: &tool.Ref, Kind: "input.request", AuthorAgent: &agent, Payload: map[string]interface{}{"description": "Review", "form": form}}
	if err, _ := store.SessionEventsCreateBatch(ctx, []model.SessionEvent{message, tool, input}); err != nil {
		t.Fatal(err)
	}
	dispatcher := &inputTestDispatcher{}
	return inputTestFixture{store: store, tokens: tokens, handler: HandlerWithReplyDispatcher(config.HTTPService{API: true, Web: true, PublicBaseURL: "https://gatehouse.example.test"}, store, dispatcher, tokens), dispatcher: dispatcher, input: input.Ref, login: login}
}

func inputTestRequest(handler http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	handler.ServeHTTP(response, request)
	return response
}

func inputTestCapability(t *testing.T, destination string) string {
	t.Helper()
	parsed, err := url.Parse(destination)
	if err != nil || parsed.Path != "/app/input" || parsed.Host != "gatehouse.example.test" || parsed.Scheme != "https" {
		t.Fatalf("invalid input destination: %v", err)
	}
	fragment, err := url.ParseQuery(parsed.RawFragment)
	if err != nil || len(fragment) != 1 || len(fragment["capability"]) != 1 || fragment.Get("capability") == "" {
		t.Fatalf("invalid input destination fragment: %v", err)
	}
	return fragment.Get("capability")
}

func TestInputHTTPLaunchPatchAndSubmit(t *testing.T) {
	ctx := context.Background()
	fixture := newInputTestFixture(t)
	open := "/api/v1/workspaces/" + fixture.input.Session.Workspace.Id + "/sessions/" + fixture.input.Session.Id + "/inputs/" + fixture.input.Id + "/open"
	unconfigured := HandlerWithReplyDispatcher(config.HTTPService{API: true, Web: true}, fixture.store, fixture.dispatcher, fixture.tokens)
	if response := inputTestRequest(unconfigured, http.MethodGet, open, fixture.login, ""); response.Code != http.StatusServiceUnavailable || response.Header().Get("Location") != "" {
		t.Fatalf("open without public base URL = %d, location %q", response.Code, response.Header().Get("Location"))
	}
	if response := inputTestRequest(fixture.handler, http.MethodGet, open, "", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("open without credentials = %d", response.Code)
	}
	cookieRequest := httptest.NewRequest(http.MethodGet, open, nil)
	cookieRequest.AddCookie(&http.Cookie{Name: "gatehouse_auth", Value: fixture.login})
	cookieRequest.Host = "internal-proxy.invalid:4283"
	cookieRequest.Header.Set("X-Forwarded-Proto", "http")
	cookieOpen := httptest.NewRecorder()
	fixture.handler.ServeHTTP(cookieOpen, cookieRequest)
	if cookieOpen.Code != http.StatusSeeOther || cookieOpen.Header().Get("Cache-Control") != "no-store" || cookieOpen.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("cookie-authenticated redirect = %d %#v", cookieOpen.Code, cookieOpen.Header())
	}
	inputTestCapability(t, cookieOpen.Header().Get("Location"))
	jsonOpen := inputTestRequest(fixture.handler, http.MethodGet, open+"?redirect=false", fixture.login, "")
	var destination struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(jsonOpen.Body.Bytes(), &destination); err != nil || jsonOpen.Code != http.StatusOK || jsonOpen.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(destination.URL, "https://gatehouse.example.test/") {
		t.Fatalf("cookie-less JSON open = %d %q (%v)", jsonOpen.Code, jsonOpen.Body.String(), err)
	}
	capability := inputTestCapability(t, destination.URL)
	missingOpen := strings.Replace(open, fixture.input.Id, "sev_00000000000000000000000009", 1)
	if response := inputTestRequest(fixture.handler, http.MethodGet, missingOpen, fixture.login, ""); response.Code != http.StatusNotFound {
		t.Fatalf("open missing input = %d", response.Code)
	}
	wrongInput := fixture.input
	wrongInput.Id = "sev_00000000000000000000000001" // tool.request, not input.request
	alice, identity := principalIdentityRefs(t, ctx, fixture.store, "alice", "gatehouse:alice")
	err, wrongCapability := fixture.tokens.MintFormInput(ctx, wrongInput, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	if response := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input", wrongCapability, ""); response.Code != http.StatusNotFound {
		t.Fatalf("capability scoped to non-input event = %d", response.Code)
	}
	if response := inputTestRequest(fixture.handler, http.MethodGet, open+"?redirect=true", fixture.login, ""); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid open option = %d", response.Code)
	}
	for _, token := range []string{"", fixture.login} {
		response := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input", token, "")
		if response.Code != http.StatusUnauthorized || response.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Fatalf("read with non-input credentials = %d", response.Code)
		}
	}
	read := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input", capability, "")
	var state struct {
		Form      inputform.Form  `json:"form"`
		Draft     json.RawMessage `json:"draft"`
		UpdatedAt *string         `json:"updated_at"`
	}
	if err := json.Unmarshal(read.Body.Bytes(), &state); err != nil || read.Code != http.StatusOK || state.Form.Title != "Review" || string(state.Draft) != `{}` || state.UpdatedAt != nil {
		t.Fatalf("read empty input = %d %s (%v)", read.Code, read.Body.String(), err)
	}
	if state.Form.Fields[1].Fields[1].Item.Fields[0].Min == nil || *state.Form.Fields[1].Fields[1].Item.Fields[0].Min != 9007199254740993 {
		t.Fatalf("read lost exact form bound: %#v", state.Form)
	}
	for _, test := range []struct {
		body string
		want int
	}{
		{`{`, http.StatusBadRequest},
		{`{"op":"set","path":["name"]}`, http.StatusBadRequest},
		{`{"op":"remove","path":["name"],"value":null}`, http.StatusBadRequest},
		{`{"op":"set","path":["name"],"value":"Ada","extra":1}`, http.StatusBadRequest},
		{`{"op":"set","path":["name"],"value":"Ada"}{}`, http.StatusBadRequest},
		{`{"op":"set","path":["profile","entries","0"],"value":"Ada"}`, http.StatusBadRequest},
		{`{"op":"set","path":["other"],"value":"Ada"}`, http.StatusBadRequest},
		{`{"op":"set","path":["agreed"],"value":null}`, http.StatusUnprocessableEntity},
		{`{"op":"set","path":["profile"],"value":{"unknown":1}}`, http.StatusUnprocessableEntity},
		{`{"op":"set","path":["profile","entries"],"value":[{"count":9007199254740992}]}`, http.StatusUnprocessableEntity},
		{`{"op":"set","path":["name"],"value":"` + strings.Repeat("a", 1<<20) + `"}`, http.StatusRequestEntityTooLarge},
	} {
		response := inputTestRequest(fixture.handler, http.MethodPatch, "/api/v1/input/draft", capability, test.body)
		if response.Code != test.want {
			t.Fatalf("patch %.120s = %d, want %d (%s)", test.body, response.Code, test.want, response.Body.String())
		}
	}
	if err, draft := fixture.store.SessionInputDraftGet(ctx, fixture.input); err != nil || draft != nil {
		t.Fatalf("invalid patches created draft = (%#v, %v)", draft, err)
	}
	patch := func(body string) {
		t.Helper()
		response := inputTestRequest(fixture.handler, http.MethodPatch, "/api/v1/input/draft", capability, body)
		if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
			t.Fatalf("patch %s = %d %s", body, response.Code, response.Body.String())
		}
	}
	patch(`{"op":"set","path":["profile"],"value":{"entries":[{}]}}`)
	read = inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input", capability, "")
	if err := json.Unmarshal(read.Body.Bytes(), &state); err != nil || string(state.Draft) != `{"profile":{"entries":[{}]}}` || state.UpdatedAt == nil || *state.UpdatedAt == "" {
		t.Fatalf("read shared incomplete draft = %d %s (%v)", read.Code, read.Body.String(), err)
	}
	if response := inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/submit", capability, ""); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("submit incomplete draft = %d %s", response.Code, response.Body.String())
	}
	patch(`{"op":"set","path":["name"],"value":"Ada"}`)
	patch(`{"op":"set","path":["profile","city"],"value":"Paris"}`)
	patch(`{"op":"set","path":["profile","entries"],"value":[{"count":9007199254740993}]}`)
	patch(`{"op":"set","path":["agreed"],"value":false}`)
	if response := inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/submit", capability, `{}`); response.Code != http.StatusBadRequest {
		t.Fatalf("submit with body = %d", response.Code)
	}
	submit := inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/submit", capability, "")
	var terminal struct {
		EventID string `json:"event_id"`
		Kind    string `json:"kind"`
	}
	if err := json.Unmarshal(submit.Body.Bytes(), &terminal); err != nil || submit.Code != http.StatusAccepted || terminal.Kind != "input.success" || !typed_id.Valid(typed_id.SessionEvent, terminal.EventID) || fixture.dispatcher.calls != 1 {
		t.Fatalf("submit = %d %s (%v), reconcile calls %d", submit.Code, submit.Body.String(), err, fixture.dispatcher.calls)
	}
	err, result := fixture.store.SessionInputResponseResultGet(ctx, model.SessionEventRef{Session: fixture.input.Session, Id: terminal.EventID})
	if err != nil || result != `{"agreed":false,"name":"Ada","profile":{"city":"Paris","entries":[{"count":9007199254740993}]}}` {
		t.Fatalf("submitted stored result = %q (%v)", result, err)
	}
	if err, draft := fixture.store.SessionInputDraftGet(ctx, fixture.input); err != nil || draft != nil {
		t.Fatalf("submit retained draft = (%#v, %v)", draft, err)
	}
	eventsPath := "/api/v1/workspaces/" + fixture.input.Session.Workspace.Id + "/sessions/" + fixture.input.Session.Id + "/events"
	for _, path := range []string{eventsPath, eventsPath + "?view=transcript"} {
		response := inputTestRequest(fixture.handler, http.MethodGet, path, fixture.login, "")
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"description":"Review"`) || strings.Contains(response.Body.String(), `"form"`) || strings.Contains(response.Body.String(), `"result"`) || strings.Contains(response.Body.String(), `9007199254740993`) {
			t.Fatalf("session events exposed private input content (%s): %d %s", path, response.Code, response.Body.String())
		}
	}
	if response := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/auth/me", capability, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("input capability authenticated regular API = %d", response.Code)
	}
	for _, operation := range []struct{ method, path, token, body string }{
		{http.MethodGet, open, fixture.login, ""},
		{http.MethodGet, "/api/v1/input", capability, ""},
		{http.MethodPatch, "/api/v1/input/draft", capability, `{"op":"remove","path":["name"]}`},
		{http.MethodPost, "/api/v1/input/cancel", capability, ""},
	} {
		if response := inputTestRequest(fixture.handler, operation.method, operation.path, operation.token, operation.body); response.Code != http.StatusConflict {
			t.Fatalf("resolved %s %s = %d", operation.method, operation.path, response.Code)
		}
	}
}

func TestInputHTTPLaunchWithNoConfiguration(t *testing.T) {
	fixture := newInputTestFixture(t)
	err, services := config.ResolveServices(configschema.GatehouseConfig{ApiVersion: "v1"})
	if err != nil || services.HTTP == nil {
		t.Fatalf("resolve default HTTP service = (%#v, %v)", services, err)
	}
	handler := HandlerWithReplyDispatcher(*services.HTTP, fixture.store, fixture.dispatcher, fixture.tokens)
	open := "/api/v1/workspaces/" + fixture.input.Session.Workspace.Id + "/sessions/" + fixture.input.Session.Id + "/inputs/" + fixture.input.Id + "/open?redirect=false"
	request := httptest.NewRequest(http.MethodGet, open, nil)
	request.Header.Set("Authorization", "Bearer "+fixture.login)
	request.Host = "untrusted.example.test"
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var destination struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &destination); err != nil || response.Code != http.StatusOK || !strings.HasPrefix(destination.URL, "http://127.0.0.1:4283/app/input#") {
		t.Fatalf("default input launch = %d %s (%v)", response.Code, response.Body.String(), err)
	}
	parsed, err := url.Parse(destination.URL)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := url.ParseQuery(parsed.RawFragment)
	if err != nil || fragment.Get("capability") == "" {
		t.Fatalf("default input launch capability = %v", err)
	}
	if read := inputTestRequest(handler, http.MethodGet, "/api/v1/input", fragment.Get("capability"), ""); read.Code != http.StatusOK {
		t.Fatalf("read form from default launch = %d %s", read.Code, read.Body.String())
	}
}

func TestInputHTTPResponderPermissionsAndCancel(t *testing.T) {
	ctx := context.Background()
	fixture := newInputTestFixture(t)
	var bob model.PrincipalRef
	if err := fixture.store.QueryRowContext(ctx, `SELECT id FROM gatehouse_principals WHERE alias = 'bob'`).Scan(&bob.Id); err != nil {
		t.Fatal(err)
	}
	identity, err := typed_id.New(typed_id.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.IdentitiesUpsertRevisions(ctx, []model.Identity{{
		Id: identity, Key: "gatehouse:bob", Principal: bob, Revision: 1, Verifiers: []interface{}{"gh-ver:invalid"}, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ExecContext(ctx, `INSERT INTO gatehouse_session_grants (id, workspace, session, role, principal, "group", enabled) VALUES (?, ?, ?, 'contributor', ?, NULL, TRUE)`, "sgr_00000000000000000000000009", fixture.input.Session.Workspace.Id, fixture.input.Session.Id, bob.Id); err != nil {
		t.Fatal(err)
	}
	err, login := fixture.tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: bob, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	open := "/api/v1/workspaces/" + fixture.input.Session.Workspace.Id + "/sessions/" + fixture.input.Session.Id + "/inputs/" + fixture.input.Id + "/open?redirect=false"
	response := inputTestRequest(fixture.handler, http.MethodGet, open, login, "")
	var destination struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &destination); err != nil || response.Code != http.StatusOK {
		t.Fatalf("contributor open = %d %s (%v)", response.Code, response.Body.String(), err)
	}
	capability := inputTestCapability(t, destination.URL)
	if response := inputTestRequest(fixture.handler, http.MethodPatch, "/api/v1/input/draft", capability, `{"op":"set","path":["name"],"value":"Bob"}`); response.Code != http.StatusNoContent {
		t.Fatalf("contributor patch = %d %s", response.Code, response.Body.String())
	}
	response = inputTestRequest(fixture.handler, http.MethodGet, open, fixture.login, "")
	if err := json.Unmarshal(response.Body.Bytes(), &destination); err != nil || response.Code != http.StatusOK {
		t.Fatalf("manager open = %d %s (%v)", response.Code, response.Body.String(), err)
	}
	managerToken := inputTestCapability(t, destination.URL)
	if read := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input", managerToken, ""); read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"name":"Bob"`) {
		t.Fatalf("shared draft across responders = %d %s", read.Code, read.Body.String())
	}
	if _, err := fixture.store.ExecContext(ctx, `UPDATE gatehouse_session_grants SET role = 'member' WHERE principal = ?`, bob.Id); err != nil {
		t.Fatal(err)
	}
	for _, denied := range []struct{ path, token string }{{open, login}, {"/api/v1/input", capability}} {
		if response := inputTestRequest(fixture.handler, http.MethodGet, denied.path, denied.token, ""); response.Code != http.StatusForbidden {
			t.Fatalf("member can read %s: %d", denied.path, response.Code)
		}
	}
	if _, err := fixture.store.ExecContext(ctx, `UPDATE gatehouse_session_grants SET enabled = FALSE WHERE principal = ?`, bob.Id); err != nil {
		t.Fatal(err)
	}
	if read := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input", capability, ""); read.Code != http.StatusForbidden {
		t.Fatalf("revoked responder can read = %d", read.Code)
	}
	if _, err := fixture.store.ExecContext(ctx, `UPDATE gatehouse_session_grants SET role = 'contributor', enabled = TRUE WHERE principal = ?`, bob.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ExecContext(ctx, `UPDATE gatehouse_identities SET enabled = FALSE WHERE id = ?`, identity); err != nil {
		t.Fatal(err)
	}
	if read := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input", capability, ""); read.Code != http.StatusUnauthorized {
		t.Fatalf("inactive responder can read = %d", read.Code)
	}
	if _, err := fixture.store.ExecContext(ctx, `UPDATE gatehouse_identities SET enabled = TRUE WHERE id = ?`, identity); err != nil {
		t.Fatal(err)
	}
	cancel := inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/cancel", capability, "")
	var terminal struct {
		EventID string `json:"event_id"`
		Kind    string `json:"kind"`
	}
	if err := json.Unmarshal(cancel.Body.Bytes(), &terminal); err != nil || cancel.Code != http.StatusAccepted || terminal.Kind != "input.failure" || fixture.dispatcher.calls != 1 {
		t.Fatalf("cancel = %d %s (%v)", cancel.Code, cancel.Body.String(), err)
	}
	err, event := fixture.store.SessionEventGet(ctx, model.SessionEventRef{Session: fixture.input.Session, Id: terminal.EventID})
	if err != nil || event == nil || event.AuthorPrincipal == nil || event.AuthorPrincipal.Ref != bob || event.Payload["code"] != "cancelled" {
		t.Fatalf("cancel event = (%#v, %v)", event, err)
	}
	if err, draft := fixture.store.SessionInputDraftGet(ctx, fixture.input); err != nil || draft != nil {
		t.Fatalf("cancel retained draft = (%#v, %v)", draft, err)
	}
	if late := inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/submit", managerToken, ""); late.Code != http.StatusConflict {
		t.Fatalf("second responder submitted cancelled input = %d", late.Code)
	}
}
