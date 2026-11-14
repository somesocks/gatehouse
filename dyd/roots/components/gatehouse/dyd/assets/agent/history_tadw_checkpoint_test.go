package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/internal/testdbos"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	_ "github.com/dbos-inc/dbos-transact-golang/dbos/driver/sqlite"
)

func TestRehydrateTADWHistoryRecordedOrderAndMembership(t *testing.T) {
	store, anchor := tadwAdapterDB(t)
	cfg := tadwAdapterConfig(10000)
	selected, err := loadTADWHistory(context.Background(), store, anchor, nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately not tree order, timestamp order, or a contiguous suffix.
	ids := []string{anchor.Id, selected.Window.EventIDs[0], selected.Window.EventIDs[2]}
	want := []tadwRenderedEvent{selected.History[6], selected.History[0], selected.History[2]}
	// No traversal is possible, and an unrelated payload cannot be decoded.
	if _, err := store.Exec(`DROP INDEX gatehouse_session_events_agent_terminals;
		DROP INDEX gatehouse_session_events_thread_order;
		UPDATE gatehouse_session_events SET payload='invalid JSON' WHERE id=?`, selected.Window.EventIDs[1]); err != nil {
		t.Fatal(err)
	}
	result, err := rehydrateTADWHistory(context.Background(), store, anchor.Session, ids, cfg)
	if err != nil {
		t.Fatal(err)
	}
	assertTADWInitialHistory(t, result)
	if !reflect.DeepEqual(result.History, want) || result.Window.BaseEventID != ids[0] || result.Window.CacheBaseMatched || result.Window.ToolingBudgetBytes != cfg.ContextMaxBytes-tadwRenderedBytes(want) {
		t.Fatal(result)
	}
	result.Window.EventIDs[0] = "mutated"
	if ids[0] != anchor.Id {
		t.Fatal("checkpoint IDs aliased")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := store.PingContext(ctx); err != nil {
		t.Fatal("sole connection retained", err)
	}
}

func TestRehydrateTADWHistoryFailuresAndExactBytes(t *testing.T) {
	for _, name := range []string{"missing", "wrong-session", "wrong-workspace", "skipped", "cancelled-agent", "bad-payload", "bad-render", "missing-file", "over-budget", "exact-fit"} {
		t.Run(name, func(t *testing.T) {
			store, anchor := tadwAdapterDB(t)
			cfg := tadwAdapterConfig(10000)
			all, err := loadTADWHistory(context.Background(), store, anchor, nil, cfg)
			if err != nil {
				t.Fatal(err)
			}
			ids := slices.Clone(all.Window.EventIDs)
			session := anchor.Session
			var query string
			switch name {
			case "missing":
				query = `DELETE FROM gatehouse_session_events WHERE id='` + anchor.Id + `'`
			case "wrong-session":
				session.Id = "other"
			case "wrong-workspace":
				session.Workspace.Id = "other"
			case "skipped":
				query = `UPDATE gatehouse_session_events SET kind='thinking.request' WHERE id='` + anchor.Id + `'`
			case "cancelled-agent":
				query = `UPDATE gatehouse_session_events SET kind='agent.failure',payload='{"code":"cancelled"}' WHERE id='` + anchor.Id + `'`
			case "bad-payload":
				query = `UPDATE gatehouse_session_events SET payload='invalid JSON' WHERE id='` + anchor.Id + `'`
			case "bad-render":
				query = `UPDATE gatehouse_session_events SET payload='{"text":42}' WHERE id='` + anchor.Id + `'`
			case "missing-file":
				query = `DELETE FROM gatehouse_session_files`
			case "over-budget":
				cfg = tadwAdapterConfig(all.Window.HistoryBytes - 1)
			case "exact-fit":
				cfg = tadwAdapterConfig(all.Window.HistoryBytes)
			}
			if query != "" {
				if _, err := store.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			result, err := rehydrateTADWHistory(context.Background(), store, session, ids, cfg)
			if name == "exact-fit" {
				if err != nil || !reflect.DeepEqual(result.History, all.History) {
					t.Fatal(result, err)
				}
				assertTADWInitialHistory(t, result)
			} else if err == nil || !reflect.DeepEqual(result, tadwInitialHistory{}) {
				t.Fatal("expected failure without partial history", result, err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := store.PingContext(ctx); err != nil {
				t.Fatal("connection retained after rehydration", err)
			}
		})
	}
}

func TestRehydrateTADWHistoryRecalculatesMetadataBytes(t *testing.T) {
	store, anchor := tadwAdapterDB(t)
	cfg := tadwAdapterConfig(10000)
	initial, err := loadTADWHistory(context.Background(), store, anchor, nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`UPDATE gatehouse_session_files SET name='renamed résumé & <longer>.txt'`); err != nil {
		t.Fatal(err)
	}
	result, err := rehydrateTADWHistory(context.Background(), store, anchor.Session, initial.Window.EventIDs, cfg)
	if err != nil {
		t.Fatal(err)
	}
	assertTADWInitialHistory(t, result)
	transcript := tadwTranscript{History: result.History}
	if !slices.Equal(result.Window.EventIDs, initial.Window.EventIDs) || !strings.Contains(transcript.String(), `name="renamed résumé &amp; &lt;longer&gt;.txt"`) || result.Window.HistoryBytes == initial.Window.HistoryBytes || result.Window.ToolingBudgetBytes != cfg.ContextMaxBytes-result.Window.HistoryBytes {
		t.Fatal("metadata not rehydrated/accounted without reselection", result)
	}
}

func TestRehydrateTADWHistoryValidationAndEmptySelection(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "w"}, Id: "s"}
	cfg := tadwAdapterConfig(10000)
	// Nil store proves malformed selections fail before acquiring a connection.
	for _, ids := range [][]string{{""}, {"id", "id"}} {
		if _, err := rehydrateTADWHistory(context.Background(), nil, session, ids, cfg); err == nil {
			t.Fatal("invalid IDs accepted", ids)
		}
	}
	if _, err := rehydrateTADWHistory(context.Background(), nil, session, nil, tadwWindowConfig{}); err == nil {
		t.Fatal("invalid config accepted")
	}
	if _, err := rehydrateTADWHistory(context.Background(), nil, model.SessionRef{}, nil, cfg); err == nil {
		t.Fatal("invalid scope accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := rehydrateTADWHistory(ctx, nil, session, nil, cfg); err == nil {
		t.Fatal("cancellation ignored")
	}
	for _, ids := range [][]string{nil, {}} {
		result, err := rehydrateTADWHistory(context.Background(), nil, session, ids, cfg)
		if err != nil || len(result.History) != 0 || result.Window.BaseEventID != "" || result.Window.HistoryBytes != 0 || result.Window.ToolingBudgetBytes != cfg.ContextMaxBytes {
			t.Fatal(result, err)
		}
	}
}

// A real process crash discards every rendered buffer, while leaving a PENDING
// request workflow and its committed ID checkpoint. Recovery must read those
// IDs, not rerun the selection callback (its indexes have been removed).
func TestTADWHistoryCheckpointProcess(t *testing.T) {
	path := os.Getenv("GATEHOUSE_TEST_TADW_CHECKPOINT_DB")
	if path == "" {
		t.Skip("subprocess helper")
	}
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	workflowContext, err := dbos.NewContext(ctx, dbos.Config{
		AppName: "gatehouse-tadw-history-test", ApplicationVersion: "tadw-history-v1", SQLiteSystemDB: store.DB,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(workflowContext, 5*time.Second)
	phase, testCase := os.Getenv("GATEHOUSE_TEST_TADW_PHASE"), os.Getenv("GATEHOUSE_TEST_TADW_CASE")
	if phase == "fresh" {
		// The first caller must reuse its existing buffer, not re-read after
		// checkpointing. Restore the name before restarting the second worker.
		if _, err := store.Exec(`CREATE TRIGGER tadw_after_checkpoint AFTER INSERT ON operation_outputs
			WHEN NEW.function_name='gatehouse.tadw-initial-history'
			BEGIN UPDATE gatehouse_session_files SET name='post-checkpoint-name'; END`); err != nil {
			t.Fatal(err)
		}
	}
	workflow := func(workflowContext dbos.Context, anchor model.SessionEventRef) (bool, error) {
		cfg := tadwAdapterConfig(10000)
		cfg.HistoryMinBytes = 1
		hints := []string{fmt.Sprintf("sev_%026d", 1)}
		if phase == "recover" {
			hints = []string{anchor.Id} // Would select a different suffix if consulted.
		}
		if testCase == "empty" {
			cfg = tadwAdapterConfig(1)
		}
		var history tadwInitialHistory
		var err error
		if strings.HasPrefix(testCase, "cache-prepared-") {
			selected := tadwPreparationTestAgent(anchor.Session)
			var prepared tadwPreparedHistory
			prepared, err = prepareTADWHistory(workflowContext, store, anchor, selected, false, cfg)
			if err == nil && (prepared.Agent != selected.Ref || prepared.Scope != tadwCheckpointTestCacheScope(t, testCase, anchor.Session)) {
				return false, fmt.Errorf("preparation lost selected workspace agent or compatibility scope")
			}
			history = prepared.History
		} else if strings.HasPrefix(testCase, "cache-") {
			history, err = checkpointTADWHistoryWithCacheHints(workflowContext, store, anchor, tadwCheckpointTestCacheScope(t, testCase, anchor.Session), cfg)
		} else {
			history, err = checkpointTADWHistory(workflowContext, store, anchor, hints, cfg)
		}
		if err != nil {
			return false, err
		}
		want, err := os.ReadFile(os.Getenv("GATEHOUSE_TEST_TADW_EXPECTED"))
		if err != nil {
			return false, err
		}
		transcript := tadwTranscript{History: history.History}
		if transcript.String() != string(want) {
			return false, fmt.Errorf("checkpoint history changed across recovery")
		}
		// Ordinary memory is reused through three rounds; neither history IDs
		// nor XML are inputs/results of the simulated thinking checkpoints.
		for round := 0; round < 3; round++ {
			if _, err := dbos.RunAsStep(workflowContext, func(context.Context) (bool, error) {
				if transcript.HistoryBytes() != history.Window.HistoryBytes {
					return false, fmt.Errorf("history accounting changed")
				}
				return true, nil
			}, dbos.WithStepName("test.tadw-thinking")); err != nil {
				return false, err
			}
			transcript.Tooling = append(transcript.Tooling, tadwRenderedEvent{ID: fmt.Sprint(round), XML: "<tool.success>TADW_PRIVATE_TOOLING</tool.success>\n"})
			if phase == "fresh" && round == 1 {
				os.Exit(0)
			}
		}
		return true, nil
	}
	dbos.RegisterWorkflow(workflowContext, workflow, dbos.WithWorkflowName("test.tadw-request"))
	if err := testdbos.Launch(workflowContext); err != nil {
		t.Fatal(err)
	}
	if phase == "fresh" {
		anchor := model.SessionEventRef{Session: model.SessionRef{Workspace: model.WorkspaceRef{Id: "w"}, Id: "s"}, Id: fmt.Sprintf("sev_%026d", 5)}
		handle, err := dbos.RunWorkflow(workflowContext, workflow, anchor, dbos.WithWorkflowID("tadw-request"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testdbos.GetResult(handle, dbos.WithHandleTimeout(15*time.Second)); err != nil {
			t.Fatal(err)
		}
		t.Fatal("fresh worker did not crash")
	}
	handle, err := dbos.RetrieveWorkflow[bool](workflowContext, "tadw-request")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := testdbos.GetResult(handle, dbos.WithHandleTimeout(15*time.Second))
	if testCase == "missing" {
		if err == nil || !strings.Contains(err.Error(), "event not found") {
			t.Fatalf("missing checkpointed event silently replaced: %v", err)
		}
	} else if err != nil || !ok {
		t.Fatal(ok, err)
	}
}

func TestTADWHistoryCheckpointRecovery(t *testing.T) {
	for _, testCase := range []string{"full", "empty", "missing", "cache-changed", "cache-missing", "cache-prepared-changed", "cache-prepared-missing"} {
		t.Run(testCase, func(t *testing.T) {
			store, anchor := tadwAdapterDB(t)
			if strings.HasPrefix(testCase, "cache-") {
				tadwCacheTestTable(t, store)
				base := model.SessionEventRef{Session: anchor.Session, Id: fmt.Sprintf("sev_%026d", 1)}
				if err := publishTADWHistoryCacheHint(context.Background(), store, tadwCheckpointTestCacheScope(t, testCase, anchor.Session), base); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload='{"text":"TADW_PRIVATE_PAYLOAD<&"}' WHERE id=?`, fmt.Sprintf("sev_%026d", 1)); err != nil {
				t.Fatal(err)
			}
			cfg := tadwAdapterConfig(10000)
			cfg.HistoryMinBytes = 1
			if testCase == "empty" {
				cfg = tadwAdapterConfig(1)
			}
			initial, err := loadTADWHistory(context.Background(), store, anchor, []string{fmt.Sprintf("sev_%026d", 1)}, cfg)
			if err != nil {
				t.Fatal(err)
			}
			var sequence int
			var name, path string
			if err := store.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path); err != nil {
				t.Fatal(err)
			}
			expected := path + ".expected"
			transcript := tadwTranscript{History: initial.History}
			if err := os.WriteFile(expected, []byte(transcript.String()), 0600); err != nil {
				t.Fatal(err)
			}
			run := func(phase string) {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTADWHistoryCheckpointProcess$", "-test.timeout=25s")
				cmd.Env = append(os.Environ(), "DBOS__APPVERSION=", "DBOS__VMID=tadw-history-test-worker",
					"GATEHOUSE_TEST_TADW_CHECKPOINT_DB="+path, "GATEHOUSE_TEST_TADW_EXPECTED="+expected,
					"GATEHOUSE_TEST_TADW_CASE="+testCase, "GATEHOUSE_TEST_TADW_PHASE="+phase)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%s worker: %v\n%s", phase, err, out)
				}
			}
			var hintsBefore []database.ContextCacheHint
			if strings.HasPrefix(testCase, "cache-") {
				// An accidental touch/publication in the fresh callback must be
				// observable even on systems with coarse wall-clock resolution.
				if _, err := store.Exec(`UPDATE gatehouse_context_cache_hints SET last_used_at=1`); err != nil {
					t.Fatal(err)
				}
				hintsBefore, err = store.ContextCacheHintsList(context.Background(), tadwCheckpointTestCacheScope(t, testCase, anchor.Session).databaseScope())
				if err != nil {
					t.Fatal(err)
				}
			}
			run("fresh")
			if strings.HasPrefix(testCase, "cache-") {
				hintsAfter, err := store.ContextCacheHintsList(context.Background(), tadwCheckpointTestCacheScope(t, testCase, anchor.Session).databaseScope())
				if err != nil || !reflect.DeepEqual(hintsBefore, hintsAfter) {
					t.Fatal("checkpointing published/touched hints", hintsBefore, hintsAfter, err)
				}
			}
			assertTADWCheckpointStorage(t, store, initial.Window.EventIDs, 2)
			var status string
			if err := store.QueryRow(`SELECT status FROM workflow_status WHERE workflow_uuid='tadw-request'`).Scan(&status); err != nil || status != "PENDING" {
				t.Fatalf("not a real pending-workflow crash: %s, %v", status, err)
			}
			var fileName string
			if err := store.QueryRow(`SELECT name FROM gatehouse_session_files`).Scan(&fileName); err != nil || fileName != "post-checkpoint-name" {
				t.Fatalf("fresh buffer reuse probe did not fire: %q, %v", fileName, err)
			}
			if _, err := store.Exec(`DROP TRIGGER tadw_after_checkpoint;
				UPDATE gatehouse_session_files SET name='résumé<&.txt';
				DROP INDEX gatehouse_session_events_agent_terminals;
				DROP INDEX gatehouse_session_events_thread_order;
				INSERT INTO gatehouse_session_events(workspace,session,id,parent,kind,author_principal,payload,created_at)
				VALUES('w','s','new-sibling',NULL,'message.text','user','invalid JSON','00');`); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(testCase, "cache-") && strings.HasSuffix(testCase, "-missing") {
				if _, err := store.Exec(`DROP TABLE gatehouse_context_cache_hints`); err != nil {
					t.Fatal(err)
				}
			}
			if strings.HasPrefix(testCase, "cache-") && strings.HasSuffix(testCase, "-changed") {
				if _, err := store.Exec(`DELETE FROM gatehouse_context_cache_hints`); err != nil {
					t.Fatal(err)
				}
				if err := publishTADWHistoryCacheHint(context.Background(), store, tadwCheckpointTestCacheScope(t, testCase, anchor.Session), anchor); err != nil {
					t.Fatal(err)
				}
			}
			if testCase == "missing" {
				if _, err := store.Exec(`DELETE FROM gatehouse_session_events WHERE id=?`, anchor.Id); err != nil {
					t.Fatal(err)
				}
			}
			run("recover")
			steps := 3
			if testCase == "missing" {
				steps = 2
			}
			assertTADWCheckpointStorage(t, store, initial.Window.EventIDs, steps)
			assertTADWDBOSPayloadAbsent(t, store)
		})
	}
}

func assertTADWCheckpointStorage(t *testing.T, store *database.Store, ids []string, thinkingSteps int) {
	t.Helper()
	rows, err := store.Query(`SELECT function_name,output FROM operation_outputs WHERE workflow_uuid='tadw-request' ORDER BY function_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	selections, thinking := 0, 0
	for rows.Next() {
		var name, output string
		if err := rows.Scan(&name, &output); err != nil {
			t.Fatal(err)
		}
		var raw []byte
		if output != "__DBOS_NIL" {
			raw, err = base64.StdEncoding.DecodeString(output)
			if err != nil {
				t.Fatal(err)
			}
		}
		switch name {
		case tadwHistoryCheckpointName:
			selections++
			var stored []string
			if raw != nil {
				if err := json.Unmarshal(raw, &stored); err != nil {
					t.Fatalf("selection persisted more than an ordered ID list: %s", raw)
				}
			}
			if !slices.Equal(stored, ids) {
				t.Fatal("checkpoint IDs/order changed", stored, ids)
			}
		case "test.tadw-thinking":
			thinking++
			if string(raw) != "true" {
				t.Fatal("thinking checkpoint contains history", string(raw))
			}
		default:
			t.Fatal("unexpected durable operation", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if selections != 1 || thinking != thinkingSteps {
		t.Fatalf("durable copies: selections=%d thinking=%d", selections, thinking)
	}
}

func assertTADWDBOSPayloadAbsent(t *testing.T, store *database.Store) {
	t.Helper()
	rows, err := store.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'gatehouse_%' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	for _, table := range tables {
		quoted := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
		rows, err := store.Query(`SELECT * FROM ` + quoted)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			for i, value := range values {
				text := fmt.Sprint(value)
				if bytes, ok := value.([]byte); ok {
					text = string(bytes)
				}
				decoded, _ := base64.StdEncoding.DecodeString(text)
				for _, marker := range []string{"TADW_PRIVATE_PAYLOAD", "TADW_PRIVATE_TOOLING", "<message "} {
					if strings.Contains(text, marker) || strings.Contains(string(decoded), marker) {
						t.Fatalf("durable payload/XML in %s.%s", table, columns[i])
					}
				}
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
}
