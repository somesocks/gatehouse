package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestGroupSecretsRespectRoundAdmissionAndLiveCredentialAccess(t *testing.T) {
	for _, scenario := range []string{"shared", "principal-only", "bot-only", "principal-revoked", "bot-revoked", "deleted"} {
		t.Run(scenario, func(t *testing.T) {
			allowed := scenario == "shared"
			started, resume := make(chan struct{}, 1), make(chan struct{})
			var calls atomic.Int32
			var secretID string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				select { case started <- struct{}{}: default: }
				select { case <-resume: case <-r.Context().Done(): return }
				w.Header().Set("Content-Type", "application/json")
				arguments, _ := json.Marshal(map[string]string{"code": `(group/secrets/read "`+secretID+`")`, "reason": "Use group credential"})
				_ = json.NewEncoder(w).Encode(map[string]any{"output": []map[string]any{{"type": "function_call", "call_id": "group-secret-read", "name": "lisp", "arguments": string(arguments)}}})
			}))
			defer server.Close()
			var once sync.Once
			release := func() { once.Do(func() { close(resume) }) }
			defer release()
			store, _, request, revokeChat := roundAccessFixture(t, server.URL, "bot")
			select { case <-started: case <-time.After(10*time.Second): t.Fatal("round did not reach the provider") }
			ctx := context.Background()
			workspace := request.Session.Workspace
			alice := principalRef(t, ctx, store, "alice")
			alias := "credentials"
			err, group := store.SystemGroupCreate(ctx, workspace.Id, database.SystemGroupInput{Alias: &alias})
			if err != nil { t.Fatal(err) }
			if err := store.GroupMemberCreate(ctx, workspace.Id, group.ID, alice.Id, authz.Manager, nil); err != nil { t.Fatal(err) }
			err, root := store.SessionEventGet(ctx, request)
			if err != nil || root == nil { t.Fatalf("request is unavailable: %v", err) }
			botID, _ := root.Payload["agent"].(string)
			bot := model.WorkspaceAgentRef{Workspace: workspace, Id: botID}
			if scenario != "principal-only" {
				if err := store.SystemGroupAgentCreate(ctx, workspace.Id, group.ID, botID); err != nil { t.Fatal(err) }
			}
			err, ring := keychain.NewKeyring(store, []config.Keychain{{ID: config.SystemKeychainID, Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_ROUND_KEYCHAIN"}}}, keychain.NewPassphraseSourceResolver())
			if err != nil { t.Fatal(err) }
			defer ring.Close()
			err, tokens := auth.Prepare(ctx, store, ring)
			if err != nil { t.Fatal(err) }
			id, err := typed_id.New(typed_id.GroupSecret)
			if err != nil { t.Fatal(err) }
			secretID = id
			ref := model.GroupSecretRef{Workspace: workspace, Group: model.GroupRef{Id: group.ID}, Id: id}
			plain := []byte("ROUND_CREDENTIAL_SENTINEL")
			err, ciphertext := tokens.Encrypt(ctx, database.GroupSecretAssociatedData(ref), plain)
			clear(plain)
			if err != nil { t.Fatal(err) }
			if err, _ := store.GroupSecretCreate(ctx, model.GroupSecret{Ref: ref, Description: "Credential", Ciphertext: ciphertext}, alice); err != nil { t.Fatal(err) }
			if scenario == "principal-revoked" || scenario == "bot-revoked" {
				if err, items := store.GroupSecretsForBotGet(ctx, workspace, alice, bot); err != nil || len(items) != 1 { t.Fatalf("shared credential not initially available: %#v %v", items, err) }
			}
			revokeChat()
			if scenario == "bot-only" || scenario == "principal-revoked" {
				if _, err := store.Exec(`UPDATE gatehouse_group_members SET enabled = FALSE WHERE workspace_id = ? AND group_id = ? AND principal_id = ?`, workspace.Id, group.ID, alice.Id); err != nil { t.Fatal(err) }
			}
			if scenario == "bot-revoked" {
				if err := store.SystemGroupAgentUpdate(ctx, workspace.Id, group.ID, botID, false, 1); err != nil { t.Fatal(err) }
			}
			if scenario == "deleted" {
				if err := store.GroupSecretRemove(ctx, ref, alice, 1); err != nil { t.Fatal(err) }
			}
			release()
			deadline := time.Now().Add(10*time.Second)
			for {
				err, events := store.SessionEventsGet(ctx, request.Session)
				if err != nil { t.Fatal(err) }
				success, failure := 0, 0
				finished := false
				for _, event := range events {
					if event.Kind == model.SessionEventKindCancelRequest { t.Fatal("resource checks recreated revocation cancellation") }
					if event.Kind == model.SessionEventKindToolSuccess {
						success++
						if event.Payload["output"] != "#<secret>" { t.Fatalf("credential result not masked: %#v", event.Payload) }
					}
					if event.Kind == model.SessionEventKindToolFailure { failure++ }
					if event.Kind == model.SessionEventKindAgentFailure { finished = true }
					payload, _ := json.Marshal(event.Payload)
					if strings.Contains(string(payload), "ROUND_CREDENTIAL_SENTINEL") { t.Fatal("credential leaked into transcript") }
				}
				if finished {
					if calls.Load() != 1 { t.Fatalf("new round started after chat revocation: %d", calls.Load()) }
					if (allowed && (success != 1 || failure != 0)) || (!allowed && (failure != 1 || success != 0)) { t.Fatalf("credential outcome: success=%d failure=%d scenario=%s", success, failure, scenario) }
					break
				}
				if time.Now().After(deadline) { t.Fatal("credential tool round did not finish") }
				time.Sleep(20*time.Millisecond)
			}
		})
	}
}
