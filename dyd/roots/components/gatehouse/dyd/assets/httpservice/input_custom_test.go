package httpservice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"gatehouse/agent"
	"gatehouse/inputform"
	"gatehouse/lisp"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func customFieldToken(t *testing.T, fixture inputTestFixture, formToken string, path []string) string {
	t.Helper()
	encoded, _ := json.Marshal(path)
	response := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input/fields/open?"+url.Values{"path": {string(encoded)}}.Encode(), formToken, "")
	var launch struct {
		URL string `json:"url"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &launch) != nil {
		t.Fatalf("custom launch = %d %s", response.Code, response.Body.String())
	}
	destination, err := url.Parse(launch.URL)
	if err != nil || destination.Path != "/app/tools/session-file-picker/" || destination.Host != "gatehouse.example.test" {
		t.Fatalf("custom launch URL = %s (%v)", launch.URL, err)
	}
	// The browser exposes the escaped fragment in window.location.hash.
	// URL.Fragment would silently decode one layer before ParseQuery and hide
	// double-encoding bugs in the launch URL.
	fragment, err := url.ParseQuery(destination.EscapedFragment())
	if err != nil || fragment.Get("version") != "1" || fragment.Get("api") != "https://gatehouse.example.test/api/v1/input/field" || fragment.Get("capability") == "" {
		t.Fatalf("custom launch fragment = %v (%v)", fragment, err)
	}
	return fragment.Get("capability")
}

func formTokenForFixture(t *testing.T, fixture inputTestFixture) string {
	t.Helper()
	open := "/api/v1/workspaces/" + fixture.input.Session.Workspace.Id + "/sessions/" + fixture.input.Session.Id + "/inputs/" + fixture.input.Id + "/open"
	response := inputTestRequest(fixture.handler, http.MethodGet, open, fixture.login, "")
	return inputTestCapability(t, response)
}

func TestCustomInputSessionFilePickerRoundTrip(t *testing.T) {
	var selected string
	err, result := lisp.Evaluate(`(import (json @native:json/v1) (input @native:gatehouse/input/v1)
  (json/decode (input/ask-json
    (input/form "Choose an attachment"
      (input/form/object "selection" "Selection"
        (input/form/custom "file" "File" "/app/tools/session-file-picker/"
          (json/object (pair "media_types" (json/array (json/string "text/plain"))))
          (input/form/custom/capabilities (list "session.file.list" "session.file.read" "session.file.upload"))))))))`, lisp.EvalOptions{HostModules: []lisp.HostModule{agent.NewInputModule(func(form inputform.Form) (error, string) {
		fixture := newInputTestFixture(t, form.Fields[0], inputform.Field{ID: "list_only", Label: "List only", Type: "custom", Optional: true, Custom: &inputform.CustomField{URL: "/app/tools/session-file-picker/", Inputs: json.RawMessage(`{}`), Capabilities: []string{inputform.SessionFileList}}})
		formToken := formTokenForFixture(t, fixture)
		fieldToken := customFieldToken(t, fixture, formToken, []string{"selection", "file"})
		listToken := customFieldToken(t, fixture, formToken, []string{"list_only"})
		read := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input/field", fieldToken, "")
		if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"media_types":["text/plain"]`) || strings.Contains(read.Body.String(), `"profile"`) || strings.Contains(read.Body.String(), `"list_only"`) {
			t.Fatalf("field read = %d %s", read.Code, read.Body.String())
		}
		for _, path := range []string{"/api/v1/input", "/api/v1/input/submit", "/api/v1/workspaces/" + fixture.input.Session.Workspace.Id + "/sessions/" + fixture.input.Session.Id + "/files"} {
			if denied := inputTestRequest(fixture.handler, http.MethodGet, path, fieldToken, ""); denied.Code != http.StatusUnauthorized && denied.Code != http.StatusMethodNotAllowed {
				t.Fatalf("field token accessed %s = %d", path, denied.Code)
			}
		}
		if denied := inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/submit", fieldToken, ""); denied.Code != http.StatusUnauthorized {
			t.Fatalf("field token submitted form = %d", denied.Code)
		}
		if denied := inputTestRequest(fixture.handler, http.MethodPatch, "/api/v1/input/field", fieldToken, `{"op":"set","path":["name"],"value":"other"}`); denied.Code != http.StatusBadRequest {
			t.Fatalf("field token changed path = %d", denied.Code)
		}
		if denied := inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/field/files", listToken, `{"name":"report.txt"}`); denied.Code != http.StatusForbidden {
			t.Fatalf("list token uploaded = %d", denied.Code)
		}
		if denied := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input/field", formToken, ""); denied.Code != http.StatusUnauthorized {
			t.Fatalf("form token used as field token = %d", denied.Code)
		}
		created := inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/field/files", fieldToken, `{"name":"report.txt","media_type":"text/plain"}`)
		var upload sessionFileCreateResponse
		if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &upload) != nil {
			t.Fatalf("custom upload = %d %s", created.Code, created.Body.String())
		}
		selected = upload.File.Ref.Id
		if put := inputTestRequest(fixture.handler, http.MethodPut, upload.UploadURL, "", "data"); put.Code != http.StatusNoContent {
			t.Fatalf("custom upload bytes = %d", put.Code)
		}
		if finished := inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/field/files/"+selected+"/finish", fieldToken, ""); finished.Code != http.StatusOK {
			t.Fatalf("custom finish = %d %s", finished.Code, finished.Body.String())
		}
		listed := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input/field/files", listToken, "")
		if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), selected) {
			t.Fatalf("custom list = %d %s", listed.Code, listed.Body.String())
		}
		if denied := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input/field/files/"+selected+"/download", listToken, ""); denied.Code != http.StatusForbidden {
			t.Fatalf("list token downloaded file = %d", denied.Code)
		}
		var download struct {
			URL string `json:"url"`
		}
		link := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input/field/files/"+selected+"/download", fieldToken, "")
		if link.Code != http.StatusOK || json.Unmarshal(link.Body.Bytes(), &download) != nil {
			t.Fatalf("custom download = %d %s", link.Code, link.Body.String())
		}
		downloadURL, err := url.Parse(download.URL)
		if err != nil {
			t.Fatal(err)
		}
		storageCredential := downloadURL.Query().Get("token")
		storageErr, storageClaims := fixture.tokens.AuthenticateStorageToken(context.Background(), storageCredential)
		expires, parseErr := time.Parse("2006-01-02T15:04:05.000Z", storageClaims.ExpiresAt)
		if storageErr != nil || parseErr != nil || storageClaims.ID != upload.File.StorageObject.Id || storageClaims.Action != "get" || storageCredential == fieldToken || time.Until(expires) > 5*time.Minute || time.Until(expires) < 4*time.Minute {
			t.Fatalf("download token scope/expiry invalid: %#v (%v, %v)", storageClaims, storageErr, parseErr)
		}
		if denied := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input/field", storageCredential, ""); denied.Code != http.StatusUnauthorized {
			t.Fatalf("storage token used as field credential=%d", denied.Code)
		}
		if bytes := inputTestRequest(fixture.handler, http.MethodGet, download.URL, "", ""); bytes.Code != http.StatusOK || bytes.Body.String() != "data" {
			t.Fatalf("custom file bytes = %d %s", bytes.Code, bytes.Body.String())
		}
		// A delegated upload/read is bound to this session, even if the principal can access another session.
		ctx := context.Background()
		err, claims := fixture.tokens.Authenticate(ctx, "Bearer "+fixture.login)
		if err != nil {
			t.Fatal(err)
		}
		otherID, err := typed_id.New(typed_id.Session)
		if err != nil {
			t.Fatal(err)
		}
		other := model.SessionRef{Workspace: fixture.input.Session.Workspace, Id: otherID}
		if err, _ := fixture.store.SessionsCreate(ctx, model.Session{Ref: other, AuthorPrincipal: &claims.Principal.Ref, Enabled: true}, claims.Principal.Ref); err != nil {
			t.Fatal(err)
		}
		otherPath := "/api/v1/workspaces/" + other.Workspace.Id + "/sessions/" + other.Id + "/files"
		otherCreated := inputTestRequest(fixture.handler, http.MethodPost, otherPath, fixture.login, `{"name":"other.txt"}`)
		var otherUpload sessionFileCreateResponse
		if otherCreated.Code != http.StatusCreated || json.Unmarshal(otherCreated.Body.Bytes(), &otherUpload) != nil {
			t.Fatalf("other upload = %d %s", otherCreated.Code, otherCreated.Body.String())
		}
		for _, action := range []struct{ method, name string }{{http.MethodPost, "finish"}, {http.MethodGet, "download"}} {
			denied := inputTestRequest(fixture.handler, action.method, "/api/v1/input/field/files/"+otherUpload.File.Ref.Id+"/"+action.name, fieldToken, "")
			if denied.Code != http.StatusNotFound {
				t.Fatalf("field token accessed other session file = %d %s", denied.Code, denied.Body.String())
			}
		}
		if saved := inputTestRequest(fixture.handler, http.MethodPatch, "/api/v1/input/field", fieldToken, `{"op":"set","value":{"file_id":"`+selected+`","exact":9007199254740993}}`); saved.Code != http.StatusNoContent {
			t.Fatalf("custom save = %d %s", saved.Code, saved.Body.String())
		}
		if sibling := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input/field", listToken, ""); strings.Contains(sibling.Body.String(), selected) {
			t.Fatal("field token read a sibling value")
		}
		if saved := inputTestRequest(fixture.handler, http.MethodPatch, "/api/v1/input/draft", formToken, `{"op":"set","path":["name"],"value":"Ada"}`); saved.Code != http.StatusNoContent {
			t.Fatalf("save name = %d", saved.Code)
		}
		submitted := inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/submit", formToken, "")
		var terminal struct {
			EventID string `json:"event_id"`
		}
		if submitted.Code != http.StatusAccepted || json.Unmarshal(submitted.Body.Bytes(), &terminal) != nil {
			t.Fatalf("custom submit = %d %s", submitted.Code, submitted.Body.String())
		}
		if late := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input/field/files", fieldToken, ""); late.Code != http.StatusConflict {
			t.Fatalf("field token survived resolution = %d", late.Code)
		}
		return fixture.store.SessionInputResponseResultGet(ctx, model.SessionEventRef{Session: fixture.input.Session, Id: terminal.EventID})
	})}})
	if err != nil || result == nil || !strings.Contains(result.String(), selected) || !strings.Contains(result.String(), `json/number "9007199254740993"`) {
		t.Fatalf("custom field result = %v (%v)", result, err)
	}
}

func TestCustomInputFieldTokenRevocation(t *testing.T) {
	fixture := newInputTestFixture(t, inputform.Field{ID: "file", Label: "File", Type: "custom", Custom: &inputform.CustomField{URL: "/app/tools/session-file-picker/", Inputs: json.RawMessage(`{}`), Capabilities: []string{inputform.SessionFileList}}})
	token := customFieldToken(t, fixture, formTokenForFixture(t, fixture), []string{"file"})
	err, claims := fixture.tokens.Authenticate(context.Background(), "Bearer "+fixture.login)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.Exec(`UPDATE gatehouse_session_grants SET role = 'member' WHERE principal = ?`, claims.Principal.Ref.Id); err != nil {
		t.Fatal(err)
	}
	if response := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input/field/files", token, ""); response.Code != http.StatusForbidden {
		t.Fatalf("revoked session permission used field token = %d", response.Code)
	}
	if _, err := fixture.store.Exec(`UPDATE gatehouse_session_grants SET role = 'manager' WHERE principal = ?`, claims.Principal.Ref.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.Exec(`UPDATE gatehouse_identities SET enabled = FALSE WHERE id = ?`, claims.Identity); err != nil {
		t.Fatal(err)
	}
	if response := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input/field", token, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked identity used field token = %d", response.Code)
	}
}
