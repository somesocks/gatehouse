package httpservice

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/model"
)

func TestSystemCredentialAPIsSelectCurrentKeyAndPreserveMetadataEdits(t *testing.T) {
	tokens, store, _ := testBearerTokens(t)
	ctx := context.Background()
	principal, identityID := principalIdentityRefs(t, ctx, store, "alice", "gatehouse:alice")
	err, token := tokens.Mint(ctx, auth.Claims{Principal: model.Principal{Ref: principal, Enabled: true}, Identity: identityID})
	if err != nil {
		t.Fatal(err)
	}
	err, ring := keychain.NewKeyring(store, []config.Keychain{{ID: config.SystemKeychainID, Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_KEYCHAIN"}}}, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer ring.Close()
	handler := HandlerWithKeyring(config.HTTPService{API: true}, store, ring, tokens)
	request := func(method, path, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1/system/"+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, response.Code, response.Body.String())
		}
		return response
	}
	request("POST", "agent-providers", `{"alias":"forbidden","protocol":"openai-responses","keychain":"default"}`, 400)
	request("POST", "storage-providers", `{"alias":"forbidden","protocol":"s3","keychain":"default"}`, 400)
	agentBody := `{"alias":"rotating-api","protocol":"openai-responses","endpoint":"https://example.test/responses","enabled":true`
	storageBody := `{"alias":"rotating-storage","protocol":"s3","endpoint":"https://example.test","region":"test","bucket":"test","access_key_id":"test","enabled":true`
	var agent database.SystemAgentProvider
	var storage database.SystemStorageProvider
	if err := json.Unmarshal(request("POST", "agent-providers", agentBody+`,"api_key":"original"}`, 200).Body.Bytes(), &agent); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(request("POST", "storage-providers", storageBody+`,"secret_access_key":"original"}`, 200).Body.Bytes(), &storage); err != nil {
		t.Fatal(err)
	}
	kdf := keychain.KDF{Salt: []byte("0123456789abcdef")}
	err, kek := keychain.DeriveKEK([]byte("test passphrase"), kdf)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(kek)
	dek := []byte("fedcba9876543210")
	err, wrapped := keychain.Seal(rand.Reader, kek, []byte("gh=v1|key=default|ver=2"), dek)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.KeychainsInsert(ctx, []model.Keychain{{Ref: model.KeychainRef{Id: config.SystemKeychainID, Version: 2}, KekKdf: kdf.String(), Key: wrapped.String(), Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	// Metadata-only changes keep the version paired with the old ciphertext.
	if err := json.Unmarshal(request("PATCH", "agent-providers/"+agent.ID, agentBody+`,"expected_revision":1}`, 200).Body.Bytes(), &agent); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(request("PATCH", "storage-providers/"+storage.ID, storageBody+`,"expected_revision":1}`, 200).Body.Bytes(), &storage); err != nil {
		t.Fatal(err)
	}
	if agent.Keychain.Version != 1 || storage.Keychain.Version != 1 {
		t.Fatal("metadata edit relabeled old ciphertext")
	}
	if err := json.Unmarshal(request("PATCH", "agent-providers/"+agent.ID, agentBody+`,"expected_revision":2,"api_key":"replacement"}`, 200).Body.Bytes(), &agent); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(request("PATCH", "storage-providers/"+storage.ID, storageBody+`,"expected_revision":2,"secret_access_key":"replacement"}`, 200).Body.Bytes(), &storage); err != nil {
		t.Fatal(err)
	}
	if agent.Keychain.Id != config.SystemKeychainID || agent.Keychain.Version != 2 || storage.Keychain.Id != config.SystemKeychainID || storage.Keychain.Version != 2 {
		t.Fatal("replacement did not use current system version")
	}
	for _, test := range []struct{ table, field, alias, aad string }{
		{"gatehouse_agent_providers", "credential", agent.Alias, "gh=v1|agent-provider|alias=" + agent.Alias},
		{"gatehouse_storage_providers", "secret_access_key", storage.Alias, "gh=v1|storage-provider|alias=" + storage.Alias},
	} {
		var ciphertext string
		if err := store.QueryRowContext(ctx, `SELECT `+test.field+` FROM `+test.table+` WHERE alias = ?`, test.alias).Scan(&ciphertext); err != nil {
			t.Fatal(err)
		}
		err, value := keychain.ParseKey(ciphertext)
		if err != nil {
			t.Fatal(err)
		}
		err, plain := keychain.Open(dek, []byte(test.aad), value)
		if err != nil || string(plain) != "replacement" {
			t.Fatalf("replacement ciphertext used the wrong key: %v", err)
		}
		clear(plain)
	}
}
