package httpservice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func newInputTestFixture(t *testing.T, extra ...inputform.Field) inputTestFixture {
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
	tool := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"}, Parent: &message.Ref, Kind: "tool.request", AuthorAgent: &agent, Payload: map[string]interface{}{"name": "lisp", "call_id": "call-1", "code": "(input/ask ...)", "reason": "Collect input"}}
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
	form.Fields = append(form.Fields, extra...)
	input := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000002"}, Parent: &tool.Ref, Kind: "input.request", AuthorAgent: &agent, Payload: map[string]interface{}{"description": "Review", "form": form}}
	if err, _ := store.SessionEventsCreateBatch(ctx, []model.SessionEvent{message, tool, input}); err != nil {
		t.Fatal(err)
	}
	dispatcher := &inputTestDispatcher{}
	return inputTestFixture{store: store, tokens: tokens, handler: HandlerWithReplyDispatcher(config.HTTPService{API: true, Web: true, PublicBaseURL: "https://gatehouse.example.test"}, store, dispatcher, tokens), dispatcher: dispatcher, input: input.Ref, login: login}
}

func TestInputFileUploadStoresCompletedSessionFileSummaries(t *testing.T) {
	limit := int64(2)
	fixture := newInputTestFixture(t, inputform.Field{ID: "attachments", Label: "Attachments", Type: "files", MaxFiles: &limit, MediaTypes: []string{"text/plain"}})
	capability := formTokenForFixture(t, fixture)
	sessionFiles := "/api/v1/workspaces/" + fixture.input.Session.Workspace.Id + "/sessions/" + fixture.input.Session.Id + "/files"
	if denied := inputTestRequest(fixture.handler, http.MethodPost, sessionFiles, capability, `{"name":"existing.txt"}`); denied.Code != http.StatusUnauthorized {
		t.Fatalf("form capability accessed ordinary session files = %d", denied.Code)
	}
	create := func(mediaType string) *httptest.ResponseRecorder {
		return inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/files", capability,
			`{"path":["attachments"],"name":"report.txt","media_type":"`+mediaType+`"}`)
	}
	if bad := create("image/png"); bad.Code != http.StatusUnprocessableEntity {
		t.Fatalf("disallowed MIME = %d", bad.Code)
	}
	created := create("text/plain")
	var upload sessionFileCreateResponse
	if err := json.Unmarshal(created.Body.Bytes(), &upload); err != nil || created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s (%v)", created.Code, created.Body.String(), err)
	}
	fileID := upload.File.Ref.Id
	fileValue := `[{"id":"` + fileID + `","name":"report.txt","size":4,"media_type":"text/plain"}]`
	patch := func(value string) *httptest.ResponseRecorder {
		return inputTestRequest(fixture.handler, http.MethodPatch, "/api/v1/input/draft", capability, `{"op":"set","path":["attachments"],"value":`+value+`}`)
	}
	existing := inputTestRequest(fixture.handler, http.MethodPost, sessionFiles, fixture.login, `{"name":"existing.txt","media_type":"text/plain"}`)
	var unrelated sessionFileCreateResponse
	if err := json.Unmarshal(existing.Body.Bytes(), &unrelated); err != nil || existing.Code != http.StatusCreated {
		t.Fatalf("create unrelated session file = %d %s (%v)", existing.Code, existing.Body.String(), err)
	}
	if put := inputTestRequest(fixture.handler, http.MethodPut, unrelated.UploadURL, "", "data"); put.Code != http.StatusNoContent {
		t.Fatalf("upload unrelated file = %d", put.Code)
	}
	if finished := inputTestRequest(fixture.handler, http.MethodPost, sessionFiles+"/"+unrelated.File.Ref.Id+"/finish", fixture.login, ""); finished.Code != http.StatusOK {
		t.Fatalf("finish unrelated file = %d", finished.Code)
	}
	if other := patch(`[{"id":"` + unrelated.File.Ref.Id + `","name":"forged.txt","size":1,"media_type":"text/plain"}]`); other.Code != http.StatusNoContent {
		t.Fatalf("reuse existing session file = %d %s", other.Code, other.Body.String())
	}
	var document struct {
		Draft struct {
			Attachments []inputform.FileSummary `json:"attachments"`
		} `json:"draft"`
	}
	if read := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input", capability, ""); json.Unmarshal(read.Body.Bytes(), &document) != nil || len(document.Draft.Attachments) != 1 || document.Draft.Attachments[0].Name != "existing.txt" || document.Draft.Attachments[0].Size != 4 {
		t.Fatalf("existing session file metadata was not canonicalized: %d %s", read.Code, read.Body.String())
	}
	if early := patch(fileValue); early.Code != http.StatusUnprocessableEntity {
		t.Fatalf("accepted unfinished upload = %d %s", early.Code, early.Body.String())
	}
	if put := inputTestRequest(fixture.handler, http.MethodPut, upload.UploadURL, "", "data"); put.Code != http.StatusNoContent {
		t.Fatalf("upload = %d %s", put.Code, put.Body.String())
	}
	finished := inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/files/"+fileID+"/finish", capability, `{"path":["attachments"]}`)
	var summary inputform.FileSummary
	if err := json.Unmarshal(finished.Body.Bytes(), &summary); err != nil || finished.Code != http.StatusOK || summary.ID != fileID || summary.Name != "report.txt" || summary.Size != 4 {
		t.Fatalf("finish = %d %s (%v)", finished.Code, finished.Body.String(), err)
	}
	if forged := patch(`[{"id":"` + fileID + `","name":"forged.txt","size":4,"media_type":"text/plain"}]`); forged.Code != http.StatusNoContent {
		t.Fatalf("failed to canonicalize file summary = %d", forged.Code)
	}
	if read := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input", capability, ""); json.Unmarshal(read.Body.Bytes(), &document) != nil || len(document.Draft.Attachments) != 1 || document.Draft.Attachments[0].Name != "report.txt" {
		t.Fatalf("file draft kept untrusted metadata: %d %s", read.Code, read.Body.String())
	}
	withExisting := `[{"id":"` + unrelated.File.Ref.Id + `","name":"existing.txt","size":4,"media_type":"text/plain"},` + strings.TrimPrefix(fileValue, "[")
	if stored := patch(withExisting); stored.Code != http.StatusNoContent {
		t.Fatalf("save existing and uploaded files = %d %s", stored.Code, stored.Body.String())
	}
	if repeated := patch(`[` + strings.TrimSuffix(strings.TrimPrefix(fileValue, "["), "]") + `,` + strings.TrimSuffix(strings.TrimPrefix(fileValue, "["), "]") + `]`); repeated.Code != http.StatusUnprocessableEntity {
		t.Fatalf("accepted duplicate IDs = %d", repeated.Code)
	}
	if result := inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/submit", capability, ""); result.Code != http.StatusUnprocessableEntity {
		t.Fatalf("submitted without other required fields = %d", result.Code)
	}
	if name := inputTestRequest(fixture.handler, http.MethodPatch, "/api/v1/input/draft", capability, `{"op":"set","path":["name"],"value":"Ada"}`); name.Code != http.StatusNoContent {
		t.Fatalf("save name = %d", name.Code)
	}
	response := inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/submit", capability, "")
	// The original fixture's optional fields need no answer. The file summary is stored as result data.
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit = %d %s", response.Code, response.Body.String())
	}
	var terminal struct {
		EventID string `json:"event_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &terminal); err != nil {
		t.Fatal(err)
	}
	resultErr, result := fixture.store.SessionInputResponseResultGet(context.Background(), model.SessionEventRef{Session: fixture.input.Session, Id: terminal.EventID})
	var answer struct {
		Attachments []inputform.FileSummary `json:"attachments"`
	}
	decodeErr := json.Unmarshal([]byte(result), &answer)
	if resultErr != nil || decodeErr != nil || len(answer.Attachments) != 2 || answer.Attachments[0].ID != unrelated.File.Ref.Id || answer.Attachments[0].Name != "existing.txt" || answer.Attachments[1].ID != fileID || answer.Attachments[1].Name != "report.txt" || answer.Attachments[1].Size != 4 {
		t.Fatalf("result = %s (%v, %v)", result, resultErr, decodeErr)
	}
	if late := create("text/plain"); late.Code != http.StatusConflict {
		t.Fatalf("upload after submit = %d", late.Code)
	}
	if err, files := fixture.store.SessionFileReferencesGet(context.Background(), fixture.input.Session, []string{fileID}); err != nil || len(files) != 1 {
		t.Fatalf("submitted upload is not a session file: %#v (%v)", files, err)
	}
}

func TestInputFileDraftRejectsAnotherSessionFile(t *testing.T) {
	fixture := newInputTestFixture(t, inputform.Field{ID: "attachments", Label: "Attachments", Type: "files"})
	ctx := context.Background()
	alice, _ := principalIdentityRefs(t, ctx, fixture.store, "alice", "gatehouse:alice")
	id, err := typed_id.New(typed_id.Session)
	if err != nil {
		t.Fatal(err)
	}
	other := model.SessionRef{Workspace: fixture.input.Session.Workspace, Id: id}
	if err, _ := fixture.store.SessionsCreate(ctx, model.Session{Ref: other, AuthorPrincipal: &alice, Enabled: true}, alice); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/workspaces/" + other.Workspace.Id + "/sessions/" + other.Id + "/files"
	created := inputTestRequest(fixture.handler, http.MethodPost, path, fixture.login, `{"name":"other.txt"}`)
	var upload sessionFileCreateResponse
	if err := json.Unmarshal(created.Body.Bytes(), &upload); err != nil || created.Code != http.StatusCreated {
		t.Fatalf("create other session file = %d %s (%v)", created.Code, created.Body.String(), err)
	}
	if put := inputTestRequest(fixture.handler, http.MethodPut, upload.UploadURL, "", "data"); put.Code != http.StatusNoContent {
		t.Fatalf("upload other session file = %d", put.Code)
	}
	if finish := inputTestRequest(fixture.handler, http.MethodPost, path+"/"+upload.File.Ref.Id+"/finish", fixture.login, ""); finish.Code != http.StatusOK {
		t.Fatalf("finish other session file = %d", finish.Code)
	}
	capability := formTokenForFixture(t, fixture)
	patch := inputTestRequest(fixture.handler, http.MethodPatch, "/api/v1/input/draft", capability,
		`{"op":"set","path":["attachments"],"value":[{"id":"`+upload.File.Ref.Id+`","name":"other.txt","size":4}]}`)
	if patch.Code != http.StatusUnprocessableEntity {
		t.Fatalf("accepted another session's file = %d %s", patch.Code, patch.Body.String())
	}
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

func inputTestCapability(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var launch map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &launch); err != nil || response.Code != http.StatusOK || len(launch) != 1 || launch["capability"] == "" || response.Header().Get("Location") != "" {
		t.Fatalf("invalid input capability response: %d %s (%v)", response.Code, response.Body.String(), err)
	}
	return launch["capability"]
}

func TestInputHTTPLaunchPatchAndSubmit(t *testing.T) {
	ctx := context.Background()
	fixture := newInputTestFixture(t)
	open := "/api/v1/workspaces/" + fixture.input.Session.Workspace.Id + "/sessions/" + fixture.input.Session.Id + "/inputs/" + fixture.input.Id + "/open"
	unconfigured := HandlerWithReplyDispatcher(config.HTTPService{API: true, Web: true}, fixture.store, fixture.dispatcher, fixture.tokens)
	capabilityWithoutBaseURL := inputTestCapability(t, inputTestRequest(unconfigured, http.MethodGet, open, fixture.login, ""))
	if read := inputTestRequest(unconfigured, http.MethodGet, "/api/v1/input", capabilityWithoutBaseURL, ""); read.Code != http.StatusOK {
		t.Fatalf("read without public base URL = %d %s", read.Code, read.Body.String())
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
	if cookieOpen.Code != http.StatusOK || cookieOpen.Header().Get("Cache-Control") != "no-store" || cookieOpen.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("cookie-authenticated open = %d %#v", cookieOpen.Code, cookieOpen.Header())
	}
	inputTestCapability(t, cookieOpen)
	jsonOpen := inputTestRequest(fixture.handler, http.MethodGet, open, fixture.login, "")
	capability := inputTestCapability(t, jsonOpen)
	if jsonOpen.Header().Get("Cache-Control") != "no-store" || jsonOpen.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("cookie-less JSON open headers = %#v", jsonOpen.Header())
	}
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
	for _, query := range []string{"?redirect=true", "?redirect=false"} {
		if response := inputTestRequest(fixture.handler, http.MethodGet, open+query, fixture.login, ""); response.Code != http.StatusBadRequest {
			t.Fatalf("removed open option %s = %d", query, response.Code)
		}
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
	open := "/api/v1/workspaces/" + fixture.input.Session.Workspace.Id + "/sessions/" + fixture.input.Session.Id + "/inputs/" + fixture.input.Id + "/open"
	request := httptest.NewRequest(http.MethodGet, open, nil)
	request.Header.Set("Authorization", "Bearer "+fixture.login)
	request.Host = "untrusted.example.test"
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	capability := inputTestCapability(t, response)
	if read := inputTestRequest(handler, http.MethodGet, "/api/v1/input", capability, ""); read.Code != http.StatusOK {
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
	open := "/api/v1/workspaces/" + fixture.input.Session.Workspace.Id + "/sessions/" + fixture.input.Session.Id + "/inputs/" + fixture.input.Id + "/open"
	response := inputTestRequest(fixture.handler, http.MethodGet, open, login, "")
	capability := inputTestCapability(t, response)
	if response := inputTestRequest(fixture.handler, http.MethodPatch, "/api/v1/input/draft", capability, `{"op":"set","path":["name"],"value":"Bob"}`); response.Code != http.StatusNoContent {
		t.Fatalf("contributor patch = %d %s", response.Code, response.Body.String())
	}
	response = inputTestRequest(fixture.handler, http.MethodGet, open, fixture.login, "")
	managerToken := inputTestCapability(t, response)
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
