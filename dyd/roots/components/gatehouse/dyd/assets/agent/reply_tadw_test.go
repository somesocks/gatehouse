package agent_test

import (
	"context"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gatehouse/agent"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/internal/testdbos"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"modernc.org/sqlite"
)

const liveTADWMarker = "TADW_PRIVATE_LIVE_HISTORY"

type tadwLiveFixture struct {
	store   *database.Store
	ring    *keychain.Keyring
	ctx     dbos.Context
	runtime *agent.SessionEventReplyRuntime
	message model.SessionEvent
	older   model.SessionEventRef
}

func newTADWLiveFixture(t *testing.T, path, endpoint, protocol string, compaction config.AgentModelCompaction, policies ...*config.WorkspaceAgentRateLimits) *tadwLiveFixture {
	t.Helper()
	t.Setenv("DBOS__APPVERSION", "")
	t.Setenv("DBOS__VMID", "tadw-live-parent")
	t.Setenv("GATEHOUSE_TEST_TADW_LIVE_KEYCHAIN", "test passphrase")
	t.Setenv("GATEHOUSE_TEST_TADW_LIVE_API_KEY", "tadw-test-key")
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: path}
	err, store := database.OpenWithCrypto(ctx, configuration, config.Crypto{DefaultKDF: "pbkdf2-hmac-sha256-v0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	chains := []config.Keychain{{ID: config.SystemKeychainID, Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_TADW_LIVE_KEYCHAIN"}}}
	err, ring := keychain.NewKeyring(store, chains, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ring.Close)
	state := config.State{
		Keychains: chains, Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}}, Principals: []config.Principal{{Alias: "alice", Enabled: true}},
		AgentProviders:  []config.AgentProvider{{Alias: "test", Revision: 1, Protocol: protocol, Endpoint: &endpoint, Sources: []config.AgentProviderAPIKeySource{"env:GATEHOUSE_TEST_TADW_LIVE_API_KEY"}, Enabled: true}},
		AgentModels:     []config.AgentModel{{Alias: "assistant", Revision: 1, ProviderAlias: "test", Model: "test-model", Parameters: `{}`, Compaction: compaction, MaxTurns: 8, MaxOutputTokens: 1024, Enabled: true}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", Alias: "assistant", ModelAlias: "assistant", Revision: 1, Default: true, Enabled: true}},
	}
	if len(policies) > 0 {
		state.WorkspaceAgents[0].RateLimits = policies[0]
	}
	err, registry := migrations.Build(configuration, admittedAgentTestState(state), ring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(ctx, store, registry); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	principal := principalRef(t, ctx, store, "alice")
	var bot string
	if err := store.QueryRow(`SELECT id FROM gatehouse_workspace_agents WHERE alias='assistant'`).Scan(&bot); err != nil {
		t.Fatal(err)
	}
	session := model.SessionRef{Workspace: workspace, Id: "ses_00000000000000000000000000"}
	title := "TADW test"
	if err, _ := store.SessionsCreate(ctx, model.Session{Ref: session, AuthorPrincipal: &principal, Name: &title, Enabled: true}, principal); err != nil {
		t.Fatal(err)
	}
	older := model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}
	if err, _ := store.SessionEventsCreate(ctx, model.SessionEvent{Ref: older, Kind: model.SessionEventKindMessageText, AuthorPrincipal: &model.Principal{Ref: principal, Enabled: true}, Payload: map[string]any{"text": "TADW_PRIVATE_OLDER_HISTORY"}}); err != nil {
		t.Fatal(err)
	}
	message := model.SessionEvent{Ref: model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"}, Kind: model.SessionEventKindMessageText, AuthorPrincipal: &model.Principal{Ref: principal, Enabled: true}, Payload: map[string]any{"text": liveTADWMarker, "agents": []string{bot}}}
	root, err := dbos.NewContext(ctx, dbos.Config{AppName: "gatehouse-tadw-live", ApplicationVersion: "tadw-live-v1", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	err, runtime := agent.NewSessionEventReplyRuntimeWithOptions(root, store, ring, agent.SessionEventReplyOptions{PollingInterval: testdbos.PollInterval})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbos.Shutdown(root, 5*time.Second) })
	return &tadwLiveFixture{store: store, ring: ring, ctx: root, runtime: runtime, message: message, older: older}
}

func (f *tadwLiveFixture) submit(t *testing.T, launch bool) {
	t.Helper()
	if launch {
		if err := testdbos.Launch(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err, _ := f.runtime.CreateMessage(context.Background(), f.message); err != nil {
		t.Fatal(err)
	}
}

func (f *tadwLiveFixture) outcome(t *testing.T) model.SessionEvent {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		err, events := f.store.SessionEventsGet(context.Background(), f.message.Ref.Session)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.Kind != model.SessionEventKindAgentSuccess && event.Kind != model.SessionEventKindAgentFailure {
				continue
			}
			var active int
			if err := f.store.QueryRow(`SELECT COUNT(*) FROM workflow_status WHERE name IN ('gatehouse.session-event-reply','gatehouse.session-agent-round') AND status IN ('PENDING','ENQUEUED','DELAYED')`).Scan(&active); err != nil {
				t.Fatal(err)
			}
			if active > 0 {
				break
			}
			if event.Kind == model.SessionEventKindAgentSuccess {
				var failed int
				if err := f.store.QueryRow(`SELECT COUNT(*) FROM workflow_status WHERE name IN ('gatehouse.session-event-reply','gatehouse.session-agent-round') AND status<>'SUCCESS'`).Scan(&failed); err != nil || failed > 0 {
					t.Fatal("successful event has unfinished or failed workflows", failed, err)
				}
			}
			return event
		}
		time.Sleep(testdbos.PollInterval)
	}
	rows, err := f.store.Query(`SELECT name,status,error FROM workflow_status`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name, status string
			var failure any
			_ = rows.Scan(&name, &status, &failure)
			t.Log(name, status, failure)
		}
	}
	t.Fatal("reply did not finish")
	return model.SessionEvent{}
}

func liveProviderReply(w http.ResponseWriter, protocol string, tool int) {
	w.Header().Set("Content-Type", "application/json")
	if tool == 0 {
		text := `{"text":"TADW completed","attachments":[]}`
		if protocol == "openai-chat-completions" {
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": text}}}})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"output": []any{map[string]any{"type": "message", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": text}}}}})
		}
		return
	}
	literal, _ := json.Marshal(fmt.Sprintf("TADW_PRIVATE_TOOL_%d_%s", tool, strings.Repeat("x", 650)))
	args, _ := json.Marshal(map[string]string{"code": string(literal), "reason": "exercise a whole-event tooling window"})
	if protocol == "openai-chat-completions" {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": fmt.Sprint("call-", tool), "type": "function", "function": map[string]any{"name": "lisp", "arguments": string(args)}}}}}}})
	} else {
		_ = json.NewEncoder(w).Encode(map[string]any{"output": []any{map[string]any{"type": "function_call", "call_id": fmt.Sprint("call-", tool), "name": "lisp", "arguments": string(args)}}})
	}
}

func liveUserContent(t *testing.T, body []byte) string {
	t.Helper()
	var decoded struct {
		Messages []struct{ Role, Content string }
		Input    []struct{ Role, Content string }
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	messages := decoded.Input
	if len(decoded.Messages) > 0 {
		messages = decoded.Messages
	}
	users := 0
	content := ""
	for _, message := range messages {
		if message.Role == "user" {
			users++
			content = message.Content
		} else if message.Role != "system" {
			t.Fatalf("native historical conversation in TADW request: %s", body)
		}
	}
	if users != 1 {
		t.Fatalf("want one user message, got %d: %s", users, body)
	}
	return content
}

func liveStepOutput(t *testing.T, value string) []byte {
	t.Helper()
	if strings.HasPrefix(value, "__DBOS_") {
		return []byte("null")
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func assertLiveTADWState(t *testing.T, store *database.Store, attempts int) {
	t.Helper()
	rows, err := store.Query(`SELECT function_name,output FROM operation_outputs ORDER BY workflow_uuid,function_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	histories, boundaries, nonempty := 0, 0, 0
	var historyIDs []string
	var idLists [][]string
	for rows.Next() {
		var name string
		var encoded *string
		if err := rows.Scan(&name, &encoded); err != nil {
			t.Fatal(err)
		}
		if encoded == nil {
			continue
		}
		output := liveStepOutput(t, *encoded)
		if strings.Contains(string(output), liveTADWMarker) || strings.Contains(string(output), "TADW_PRIVATE_TOOL_") || strings.Contains(string(output), "<message ") {
			t.Fatal("transcript in DBOS", name, string(output))
		}
		var list []string
		if err := json.Unmarshal(output, &list); err == nil && len(list) > 0 {
			idLists = append(idLists, list)
		}
		switch name {
		case "DBOS.getWorkflowSteps":
			t.Fatal("checkpoint listing was persisted")
		case "gatehouse.tadw-initial-history":
			histories++
			var ids []string
			if err := json.Unmarshal(output, &ids); err != nil || len(ids) == 0 {
				t.Fatal("invalid history checkpoint", string(output), err)
			}
			historyIDs = ids
		case "gatehouse.tadw-tooling-window":
			boundaries++
			var boundary string
			if err := json.Unmarshal(output, &boundary); err != nil {
				t.Fatal(err)
			}
			if boundary != "" {
				nonempty++
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if histories != 1 || boundaries != attempts {
		t.Fatalf("history checkpoints=%d tooling checkpoints=%d, want 1/%d", histories, boundaries, attempts)
	}
	if attempts >= 4 && nonempty < 2 {
		t.Fatalf("expected multiple rollovers, got %d nonempty decisions", nonempty)
	}
	copies := 0
	for _, list := range idLists {
		if slices.Equal(list, historyIDs) {
			copies++
		}
	}
	if copies != 1 {
		t.Fatal("initial history list copies in DBOS", copies)
	}
	rows.Close()
	workflows, err := store.Query(`SELECT inputs,output,error FROM workflow_status`)
	if err != nil {
		t.Fatal(err)
	}
	defer workflows.Close()
	for workflows.Next() {
		var input, output, failure *string
		if err := workflows.Scan(&input, &output, &failure); err != nil {
			t.Fatal(err)
		}
		for _, field := range []*string{input, output, failure} {
			if field == nil {
				continue
			}
			text := *field
			if decoded, err := base64.StdEncoding.DecodeString(text); err == nil {
				text = string(decoded)
			}
			if strings.Contains(text, liveTADWMarker) || strings.Contains(text, "TADW_PRIVATE_TOOL_") || strings.Contains(text, "<message ") {
				t.Fatal("transcript in workflow input/result", text)
			}
		}
	}
	if err := workflows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestTADWNormalReplyToolingAndRetry(t *testing.T) {
	for _, protocol := range []string{"openai-chat-completions", "openai-responses"} {
		t.Run(protocol, func(t *testing.T) {
			var calls atomic.Int32
			var mu sync.Mutex
			var bodies [][]byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				mu.Lock()
				bodies = append(bodies, body)
				mu.Unlock()
				if r.Header.Get("Authorization") != "Bearer tadw-test-key" {
					t.Error("incorrect credentials")
				}
				n := int(calls.Add(1))
				if n == 1 {
					w.Header().Set("retry-after-ms", "1")
					http.Error(w, `{"error":{"code":"rate_limit_exceeded"}}`, http.StatusTooManyRequests)
					return
				}
				tool := n - 1
				if tool > 3 {
					tool = 0
				}
				liveProviderReply(w, protocol, tool)
			}))
			defer server.Close()
			f := newTADWLiveFixture(t, filepath.Join(t.TempDir(), "live.sqlite"), server.URL, protocol, config.AgentModelCompaction{TADW: &config.TADWConfig{ContextMaxBytes: 3200, HistoryMaxBytes: 1024, HistoryMinBytes: 1}})
			f.submit(t, true)
			outcome := f.outcome(t)
			if outcome.Kind != model.SessionEventKindAgentSuccess {
				_, events := f.store.SessionEventsGet(context.Background(), f.message.Ref.Session)
				for _, event := range events {
					t.Log(event.Kind, event.Payload)
				}
				t.Fatalf("reply failed: %#v", outcome.Payload)
			}
			if calls.Load() != 5 {
				t.Fatal("provider attempts", calls.Load())
			}
			mu.Lock()
			requests := append([][]byte(nil), bodies...)
			mu.Unlock()
			for index, body := range requests {
				content := liveUserContent(t, body)
				if !strings.Contains(content, liveTADWMarker) || strings.Count(content, "<agent.request ") != 1 || len([]byte(content)) > 3200 {
					t.Fatalf("attempt %d invalid transcript: %s", index, content)
				}
				if index > 1 && !strings.Contains(content, fmt.Sprint("TADW_PRIVATE_TOOL_", index-1, "_")) {
					t.Fatalf("attempt %d lost newest tooling: %s", index, content)
				}
			}
			if strings.Contains(liveUserContent(t, requests[len(requests)-1]), "TADW_PRIVATE_TOOL_1_") {
				t.Fatal("old tooling prefix was not dropped")
			}
			assertLiveTADWState(t, f.store, 4)
			var hints, contexts int
			if err := f.store.QueryRow(`SELECT COUNT(*) FROM gatehouse_context_cache_hints`).Scan(&hints); err != nil {
				t.Fatal(err)
			}
			if err := f.store.QueryRow(`SELECT COUNT(*) FROM gatehouse_agent_contexts`).Scan(&contexts); err != nil {
				t.Fatal(err)
			}
			if hints != 0 || contexts != 0 {
				t.Fatal("unexpected hint publication or MCMTR checkpoints", hints, contexts)
			}
		})
	}
}

func TestTADWRuntimeRecoveryProcess(t *testing.T) {
	path := os.Getenv("GATEHOUSE_TEST_TADW_LIVE_PATH")
	if path == "" {
		t.Skip("subprocess helper")
	}
	point := os.Getenv("GATEHOUSE_TEST_TADW_LIVE_CRASH")
	if err := sqlite.RegisterScalarFunction("gh_tadw_live_crash", 0, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) { os.Exit(0); return nil, nil }); err != nil {
		t.Fatal(err)
	}
	err, store := database.OpenWithCrypto(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: path}, config.Crypto{DefaultKDF: "pbkdf2-hmac-sha256-v0"})
	if err != nil {
		t.Fatal(err)
	}
	err, ring := keychain.NewKeyring(store, []config.Keychain{{ID: config.SystemKeychainID, Sources: []config.KeychainPassphraseSource{"env:GATEHOUSE_TEST_TADW_LIVE_KEYCHAIN"}}}, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	trigger := `CREATE TRIGGER tadw_live_crash BEFORE INSERT ON operation_outputs WHEN NEW.function_name='gatehouse.agent-rate-limit-claim' BEGIN SELECT gh_tadw_live_crash(); END`
	if point == "decision" {
		trigger = `CREATE TRIGGER tadw_live_crash BEFORE INSERT ON gatehouse_session_events WHEN NEW.kind='thinking.success' BEGIN SELECT gh_tadw_live_crash(); END`
	}
	if point == "rounds" {
		trigger = `CREATE TRIGGER tadw_live_crash BEFORE INSERT ON operation_outputs WHEN NEW.function_name='gatehouse.agent-rate-limit-claim' AND (SELECT COUNT(*) FROM operation_outputs WHERE function_name='gatehouse.agent-provider-execute')=3 BEGIN SELECT gh_tadw_live_crash(); END`
	}
	if point == "application" {
		trigger = `CREATE TRIGGER tadw_live_crash BEFORE INSERT ON operation_outputs WHEN NEW.function_name='gatehouse.agent-provider-execute' BEGIN SELECT gh_tadw_live_crash(); END`
	}
	if point == "completed" {
		trigger = `CREATE TRIGGER tadw_live_crash BEFORE UPDATE ON workflow_status WHEN NEW.name='gatehouse.session-agent-round' AND NEW.status='SUCCESS' BEGIN SELECT gh_tadw_live_crash(); END`
	}
	if _, err := store.Exec(trigger); err != nil {
		t.Fatal(err)
	}
	root, err := dbos.NewContext(context.Background(), dbos.Config{AppName: "gatehouse-tadw-live", ApplicationVersion: "tadw-live-v1", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GATEHOUSE_TEST_TADW_LIVE_LEGACY") == "true" {
		workflow, err := agent.RegisterLegacyReplyRoundForTest(root, store, ring)
		if err != nil {
			t.Fatal(err)
		}
		if err := testdbos.Launch(root); err != nil {
			t.Fatal(err)
		}
		var session, workspace, request string
		if err := store.QueryRow(`SELECT workspace,session,id FROM gatehouse_session_events WHERE kind='agent.request' LIMIT 1`).Scan(&workspace, &session, &request); err != nil {
			t.Fatal(err)
		}
		ref := model.SessionEventRef{Session: model.SessionRef{Workspace: model.WorkspaceRef{Id: workspace}, Id: session}, Id: request}
		if err := workflow(ref); err != nil {
			t.Fatal(err)
		}
		select {}
	}
	if err, _ := agent.NewSessionEventReplyRuntimeWithOptions(root, store, ring, agent.SessionEventReplyOptions{PollingInterval: testdbos.PollInterval}); err != nil {
		t.Fatal(err)
	}
	if err := testdbos.Launch(root); err != nil {
		t.Fatal(err)
	}
	select {}
}

func TestTADWNormalReplyCrashRecovery(t *testing.T) {
	for _, test := range []struct {
		name, point    string
		change, legacy bool
		before, after  int32
	}{
		{"boundary", "boundary", false, false, 0, 1},
		{"uncommitted decision", "decision", false, false, 1, 2},
		{"application decision before step checkpoint", "application", false, false, 1, 1},
		{"algorithm change before step checkpoint", "application", true, false, 1, 1},
		{"disabled provider before step checkpoint", "application", false, false, 1, 1},
		{"disabled binding before step checkpoint", "application", false, false, 1, 1},
		{"committed retry", "application", false, false, 1, 2},
		{"committed tool decision", "application", false, false, 1, 2},
		{"tooling rollover across restart", "rounds", false, false, 3, 4},
		{"completed decision", "completed", false, false, 1, 1},
		{"completed decision with rate wait", "completed", false, false, 1, 1},
		{"algorithm change before provider", "boundary", true, false, 0, 0},
		{"algorithm change after decision", "completed", true, false, 1, 1},
		{"disabled provider after decision", "completed", false, false, 1, 1},
		{"disabled binding after decision", "completed", false, false, 1, 1},
		{"legacy MCMTR admission", "completed", true, true, 1, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			var mu sync.Mutex
			var bodies [][]byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				mu.Lock()
				bodies = append(bodies, body)
				mu.Unlock()
				call := calls.Add(1)
				if test.name == "committed retry" && call == 1 {
					w.Header().Set("retry-after-ms", "1")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = io.WriteString(w, `{"error":{"code":"rate_limit_exceeded","message":"retry"}}`)
					return
				}
				tool := 0
				if test.point == "rounds" && call <= 3 {
					tool = int(call)
				}
				if test.name == "committed tool decision" && call == 1 {
					tool = 1
				}
				liveProviderReply(w, "openai-responses", tool)
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "recovery.sqlite")
			compaction := config.AgentModelCompaction{TADW: &config.TADWConfig{ContextMaxBytes: 8192, HistoryMaxBytes: 2048, HistoryMinBytes: 1024}}
			if test.legacy {
				compaction = defaultAgentModelCompaction
			}
			if test.point == "rounds" {
				compaction.TADW.ContextMaxBytes = 3200
			}
			var policy *config.WorkspaceAgentRateLimits
			if strings.Contains(test.name, "rate wait") {
				policy = liveTADWRatePolicy()
			}
			f := newTADWLiveFixture(t, path, server.URL, "openai-responses", compaction, policy)
			f.submit(t, false)
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			process := exec.Command(executable, "-test.run=^TestTADWRuntimeRecoveryProcess$")
			process.Env = append(os.Environ(), "GATEHOUSE_TEST_TADW_LIVE_PATH="+path, "GATEHOUSE_TEST_TADW_LIVE_CRASH="+test.point, fmt.Sprintf("GATEHOUSE_TEST_TADW_LIVE_LEGACY=%t", test.legacy), "DBOS__VMID=tadw-live-crashed")
			done := make(chan error, 1)
			if err := process.Start(); err != nil {
				t.Fatal(err)
			}
			defer process.Process.Kill()
			go func() { done <- process.Wait() }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(15 * time.Second):
				t.Fatal("worker did not reach crash boundary")
			}
			if calls.Load() != test.before {
				t.Fatal("calls before crash", calls.Load(), test.before)
			}
			if _, err := f.store.Exec(`DROP TRIGGER tadw_live_crash; DROP TABLE gatehouse_context_cache_hints; DROP INDEX gatehouse_session_events_thread_order; DROP INDEX gatehouse_session_events_agent_terminals`); err != nil {
				t.Fatal(err)
			}
			if test.legacy {
				if _, err := f.store.Exec(`UPDATE gatehouse_agent_models SET compaction='{"algorithm":"tadw","context_max_bytes":8192,"history_max_bytes":2048,"history_min_bytes":1024}'`); err != nil {
					t.Fatal(err)
				}
			} else {
				text := "UPDATED_HISTORY_AFTER_CRASH"
				if test.point == "completed" || (test.point == "application" && test.name != "committed retry" && test.name != "committed tool decision") {
					text = strings.Repeat("oversized history ", 300)
				}
				if _, err := f.store.Exec(`UPDATE gatehouse_session_events SET payload=json_object('text',?) WHERE id=?`, text, f.older.Id); err != nil {
					t.Fatal(err)
				}
				if test.change {
					if _, err := f.store.Exec(`UPDATE gatehouse_agent_models SET compaction='{"algorithm":"mcmtr","history_bytes":98304,"buffer_bytes":49152}'`); err != nil {
						t.Fatal(err)
					}
				}
			}
			if strings.HasPrefix(test.name, "disabled provider ") {
				if _, err := f.store.Exec(`UPDATE gatehouse_agent_providers SET enabled=FALSE`); err != nil {
					t.Fatal(err)
				}
			}
			if strings.HasPrefix(test.name, "disabled binding ") {
				if _, err := f.store.Exec(`UPDATE gatehouse_workspace_agents SET enabled=FALSE,"default"=FALSE`); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("DBOS__VMID", "tadw-live-recovered")
			recovered, err := dbos.NewContext(context.Background(), dbos.Config{AppName: "gatehouse-tadw-live", ApplicationVersion: "tadw-live-v1", SQLiteSystemDB: f.store.DB})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { dbos.Shutdown(recovered, 5*time.Second) })
			if err, _ := agent.NewSessionEventReplyRuntimeWithOptions(recovered, f.store, f.ring, agent.SessionEventReplyOptions{PollingInterval: testdbos.PollInterval}); err != nil {
				t.Fatal(err)
			}
			if err := testdbos.Launch(recovered); err != nil {
				t.Fatal(err)
			}
			var pending []string
			rows, err := f.store.Query(`SELECT workflow_uuid FROM workflow_status WHERE status='PENDING'`)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					t.Fatal(err)
				}
				pending = append(pending, id)
			}
			rows.Close()
			if len(pending) == 0 {
				t.Fatal("crash left no pending workflows")
			}
			if _, err := dbos.ResumeWorkflows[any](recovered, pending); err != nil {
				t.Fatal(err)
			}
			outcome := f.outcome(t)
			want := model.SessionEventKindAgentSuccess
			if test.change && test.point == "boundary" {
				want = model.SessionEventKindAgentFailure
			}
			if outcome.Kind != want {
				t.Fatalf("outcome=%s want=%s payload=%#v", outcome.Kind, want, outcome.Payload)
			}
			if calls.Load() != test.after {
				t.Fatal("calls after recovery", calls.Load(), test.after)
			}
			if want == model.SessionEventKindAgentSuccess {
				var executor string
				if err := f.store.QueryRow(`SELECT executor_id FROM workflow_status WHERE name='gatehouse.session-agent-round' LIMIT 1`).Scan(&executor); err != nil || executor != "tadw-live-recovered" {
					t.Fatal("round was not completed on the recovery executor", executor, err)
				}
			}
			if !test.legacy {
				thinkingRounds := 1
				if test.point == "rounds" {
					thinkingRounds = 4
				}
				if test.name == "committed tool decision" {
					thinkingRounds = 2
				}
				assertLiveTADWState(t, f.store, thinkingRounds)
				if (test.point == "boundary" || test.point == "decision" || test.point == "rounds" || test.name == "committed retry" || test.name == "committed tool decision") && !test.change {
					mu.Lock()
					body := bodies[len(bodies)-1]
					mu.Unlock()
					if !strings.Contains(liveUserContent(t, body), "UPDATED_HISTORY_AFTER_CRASH") {
						t.Fatal("recovery did not render authoritative history")
					}
				}
			} else {
				var count int
				if err := f.store.QueryRow(`SELECT COUNT(*) FROM operation_outputs WHERE function_name='gatehouse.tadw-initial-history'`).Scan(&count); err != nil || count != 0 {
					t.Fatal("legacy request changed algorithm", count, err)
				}
			}
		})
	}
}
