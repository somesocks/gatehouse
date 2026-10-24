package httpservice

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/model"
)

func TestStorageCapabilitiesRequireCurrentWorkspaceAdmission(t *testing.T) {
	tokens, store, workspaces := testBearerTokens(t)
	ctx := context.Background()
	alice, identity := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, login := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: alice, Enabled: true}, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.HTTPService{API: true}, store, tokens)
	request := func(method, path, body string, authenticated bool, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if authenticated {
			r.Header.Set("Authorization", "Bearer "+login)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w
	}
	workspace := workspaces["engineering"]
	var chat sessionResponse
	if err := json.Unmarshal(request("POST", "/api/v1/workspaces/"+workspace.Id+"/sessions", `{}`, true, 201).Body.Bytes(), &chat); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/workspaces/" + workspace.Id + "/sessions/" + chat.ID + "/files"
	start := func() sessionFileCreateResponse {
		t.Helper()
		var upload sessionFileCreateResponse
		if err := json.Unmarshal(request("POST", base, `{"name":"private.txt"}`, true, 201).Body.Bytes(), &upload); err != nil {
			t.Fatal(err)
		}
		return upload
	}
	upload := start()
	request("PUT", upload.UploadURL, "private-content", false, 204)
	request("POST", base+"/"+upload.File.Ref.Id+"/finish", "", true, 200)
	download := request("GET", base+"/"+upload.File.Ref.Id+"/download", "", true, 307).Header().Get("Location")
	if got := request("GET", download, "", false, 200).Body.String(); got != "private-content" {
		t.Fatalf("download content: %q", got)
	}
	pending := start()
	if err, removed := store.SystemWorkspacePrincipalGrantsRemove(ctx, workspace.Id, alice.Id); err != nil || !removed {
		t.Fatalf("remove admission: %t %v", removed, err)
	}
	request("GET", download, "", false, 404)
	request("PUT", pending.UploadURL, "must-not-upload", false, 404)
	if err, _ := store.SystemWorkspacePrincipalGrantCreate(ctx, workspace.Id, alice.Id, authz.Guest); err != nil {
		t.Fatal(err)
	}
	request("GET", download, "", false, 200)
	request("PUT", pending.UploadURL, "restored-upload", false, 204)
}
