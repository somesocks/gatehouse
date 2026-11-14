package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/internal/testdbos"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

type tadwHandoffTestInput struct {
	Context  tadwRoundHandoff
	Thinking model.SessionEventRef
}
type tadwHandoffTestResult struct {
	Outcome model.SessionEventRef
	Context tadwToolingHandoff
}
type tadwHandoffTestExpectation struct {
	Limits   tadwWindowConfig
	IDs      []string
	Attempts []tadwTideExpectation
}

func tadwHandoffRoot(t *testing.T, store *database.Store) dbos.Context {
	t.Helper()
	root, err := dbos.NewContext(context.Background(), dbos.Config{AppName: "tadw-handoff-test", ApplicationVersion: "tadw-handoff-v1", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbos.Shutdown(root, 5*time.Second) })
	return root
}
func tadwHandoffTestWorker(t *testing.T, root dbos.Context, store *database.Store) *tadwHandoffWorker {
	t.Helper()
	worker, err := newTADWHandoffWorker(root, store)
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

func TestTADWHandoffProcess(t *testing.T) {
	path := os.Getenv("GATEHOUSE_TEST_TADW_HANDOFF_DB")
	if path == "" {
		t.Skip("subprocess helper")
	}
	phase, testCase := os.Getenv("GATEHOUSE_TEST_TADW_HANDOFF_PHASE"), os.Getenv("GATEHOUSE_TEST_TADW_HANDOFF_CASE")
	raw, err := os.ReadFile(path + ".handoff")
	if err != nil {
		t.Fatal(err)
	}
	var expected tadwHandoffTestExpectation
	if err := json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	err, store := database.Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	root := tadwHandoffRoot(t, store)
	// These independent worker objects share the DBOS database, never memory.
	// Attempt 40 uses B to exercise a cold cross-worker read before the crash.
	a, b := tadwHandoffTestWorker(t, root, store), tadwHandoffTestWorker(t, root, store)
	request := model.SessionEventRef{Session: model.SessionRef{Workspace: model.WorkspaceRef{Id: "w"}, Id: "s"}, Id: fmt.Sprintf("sev_%026d", 10)}
	ref := func(id int) model.SessionEventRef { return tadwToolingThinking(request, id) }
	selected := tadwPreparationTestAgent(request.Session)
	if phase == "fresh" {
		if _, err := store.Exec(`CREATE TRIGGER handoff_history_probe AFTER INSERT ON operation_outputs WHEN NEW.function_name='gatehouse.tadw-initial-history'
   BEGIN UPDATE gatehouse_session_files SET name='POST_CHECKPOINT_FILE_NAME'; END;
   CREATE TRIGGER handoff_tooling_probe AFTER INSERT ON operation_outputs WHEN NEW.function_name='gatehouse.tadw-tooling-window'
   BEGIN UPDATE gatehouse_session_files SET name='POST_CHECKPOINT_FILE_NAME'; END;`); err != nil {
			t.Fatal(err)
		}
	}
	owner := func(ctx dbos.Context, request model.SessionEventRef) (tadwHistoryHandoff, error) {
		handoff, err := a.Prepare(ctx, request, ref(5), selected, false, expected.Limits)
		if err != nil {
			return tadwHistoryHandoff{}, err
		}
		if phase == "fresh" {
			// The checkpoint trigger changed metadata. A fresh callback must seed its
			// already-rendered history, not hydrate it again after persistence.
			history, err := a.load(ctx, handoff, expected.Limits)
			if err != nil || !strings.Contains((tadwTranscript{History: history.History}).String(), "résumé&lt;&amp;.txt") {
				return tadwHistoryHandoff{}, fmt.Errorf("fresh history buffer not reused: %v", err)
			}
			if _, err := store.ExecContext(ctx, `UPDATE gatehouse_session_files SET name='résumé<&.txt'`); err != nil {
				return tadwHistoryHandoff{}, err
			}
		}
		return handoff, nil
	}
	round := func(ctx dbos.Context, input tadwHandoffTestInput) (tadwHandoffTestResult, error) {
		thinking, err := dbos.RunAsStep(ctx, func(context.Context) (model.SessionEventRef, error) { return input.Thinking, nil }, dbos.WithStepName("gatehouse.agent-round-prepare"))
		if err != nil {
			return tadwHandoffTestResult{}, err
		}
		worker := a
		if thinking == ref(40) {
			worker = b
		}
		window, err := worker.Round(ctx, input.Context, thinking, selected, false, expected.Limits)
		if err != nil {
			return tadwHandoffTestResult{}, err
		}
		index := -1
		for i, id := range []int{11, 40, 60, 80, 90} {
			if thinking == ref(id) {
				index = i
			}
		}
		if phase == "fresh" && thinking == ref(80) && testCase != "full" && testCase != "completed-history-missing" {
			os.Exit(0)
		}
		// Prepare only inside the provider callback: a checkpointed provider must
		// not reconstruct XML even if its child crashed before returning.
		firstPreparation := true
		outcome, err := dbos.RunAsStep(ctx, func(stepCtx context.Context) (model.SessionEventRef, error) {
			if err := window.Prepare(stepCtx); err != nil {
				return model.SessionEventRef{}, err
			}
			want := expected.Attempts[index]
			if window.Window.FromEventID != want.From || window.Window.Transcript.String() != want.XML {
				return model.SessionEventRef{}, fmt.Errorf("attempt %d lost its recorded window", index)
			}
			content, _ := tadwProviderUserContent(t, window.Provider)
			if content != want.XML {
				return model.SessionEventRef{}, fmt.Errorf("assembled provider input changed")
			}
			if phase == "fresh" && firstPreparation {
				var name string
				if err := store.QueryRowContext(stepCtx, `SELECT name FROM gatehouse_session_files`).Scan(&name); err != nil || name != "POST_CHECKPOINT_FILE_NAME" {
					return model.SessionEventRef{}, fmt.Errorf("fresh window probe failed: %q %v", name, err)
				}
				if _, err := store.ExecContext(stepCtx, `UPDATE gatehouse_session_files SET name='résumé<&.txt'`); err != nil {
					return model.SessionEventRef{}, err
				}
			}
			firstPreparation = false
			var calls int
			if err := store.QueryRowContext(stepCtx, `INSERT INTO gatehouse_handoff_calls(id,calls) VALUES(?,1) ON CONFLICT(id) DO UPDATE SET calls=calls+1 RETURNING calls`, thinking.Id).Scan(&calls); err != nil {
				return model.SessionEventRef{}, err
			}
			if thinking == ref(11) && calls == 1 {
				return model.SessionEventRef{}, errors.New("simulated transient provider failure")
			}
			if encoded, err := json.Marshal(window.Provider.body()); err != nil || len(encoded) == 0 {
				return model.SessionEventRef{}, fmt.Errorf("empty provider request: %v", err)
			}
			outcome := thinking
			outcome.Id = fmt.Sprintf("sev_%026d", []int{13, 41, 61, 81, 91}[index])
			if _, err := store.ExecContext(stepCtx, `INSERT INTO gatehouse_session_events(workspace,session,id,parent,kind,author_agent,payload,created_at) VALUES('w','s',?,?,'thinking.success',?,'{"text":"TADW_PRIVATE_PROVIDER"}',?)`, outcome.Id, thinking.Id, selected.Ref.Id, outcome.Id); err != nil {
				return model.SessionEventRef{}, err
			}
			return outcome, nil
		}, dbos.WithStepName("test.handoff-provider"), dbos.WithStepMaxRetries(1), dbos.WithStepBaseInterval(time.Millisecond))
		if err != nil {
			return tadwHandoffTestResult{}, err
		}
		if phase == "fresh" && thinking == ref(80) {
			os.Exit(0)
		} // after provider checkpoint, before child result
		return tadwHandoffTestResult{Outcome: outcome, Context: window.Handoff}, nil
	}
	parent := func(ctx dbos.Context, request model.SessionEventRef) (bool, error) {
		handle, err := dbos.RunWorkflow(ctx, owner, request, dbos.WithWorkflowID(tadwHistoryOwnerWorkflowID(request)))
		if err != nil {
			return false, err
		}
		history, err := testdbos.GetResult(handle)
		if err != nil {
			return false, err
		}
		defer a.Release(history)
		defer b.Release(history)
		var prior *tadwToolingHandoff
		var earlier tadwToolingHandoff
		for _, attempt := range []int{11, 40, 60, 80, 90} {
			child, err := dbos.RunWorkflow(ctx, round, tadwHandoffTestInput{Context: tadwRoundHandoff{History: history, Previous: prior}, Thinking: ref(attempt)}, dbos.WithWorkflowID("handoff-round:"+strconv.Itoa(attempt)))
			if err != nil {
				return false, err
			}
			result, err := testdbos.GetResult(child)
			if err != nil {
				return false, err
			}
			if attempt == 40 {
				earlier = result.Context
			}
			if phase == "fresh" && attempt == 60 {
				bad := result.Context
				bad.Checkpoint.WorkflowID = earlier.Checkpoint.WorkflowID
				if _, err := a.previousBoundary(ctx, tadwRoundHandoff{History: history, Previous: &bad}, ref(80), &result.Outcome, selected); err == nil {
					return false, fmt.Errorf("accepted a boundary from a different admitted round")
				}
				output, err := a.output(ctx, earlier.Checkpoint, tadwToolingCheckpointName)
				if err != nil {
					return false, err
				}
				var boundary string
				if err := json.Unmarshal([]byte(output.(string)), &boundary); err != nil {
					return false, err
				}
				frozen, err := a.load(ctx, history, expected.Limits)
				if err != nil {
					return false, err
				}
				old, err := loadTADWContextFrom(ctx, store, frozen, request, selected.Ref, ref(40), expected.Limits, boundary)
				if err != nil || old.String() != expected.Attempts[1].XML {
					return false, fmt.Errorf("earlier reference lost its window after rollover: %v", err)
				}
			}
			prior = &result.Context
			if attempt == 80 && testCase == "completed-history-missing" {
				break
			}
		}
		return true, nil
	}
	dbos.RegisterWorkflow(root, owner, dbos.WithWorkflowName("test.handoff-owner"))
	dbos.RegisterWorkflow(root, round, dbos.WithWorkflowName("test.handoff-round"))
	dbos.RegisterWorkflow(root, parent, dbos.WithWorkflowName("test.handoff-parent"))
	if phase == "recover" {
		// The original process is confirmed dead. Explicitly re-enqueue its
		// pending parent/child through the public SDK onto a different executor.
		if _, err := dbos.ResumeWorkflows[any](root, []string{"handoff-parent", "handoff-round:80"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := testdbos.Launch(root); err != nil {
		t.Fatal(err)
	}
	if phase == "fresh" {
		handle, err := dbos.RunWorkflow(root, parent, request, dbos.WithWorkflowID("handoff-parent"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testdbos.GetResult(handle, dbos.WithHandleTimeout(15*time.Second)); err != nil {
			t.Fatal(err)
		}
		t.Fatal("worker did not crash")
	}
	handle, err := dbos.RetrieveWorkflow[bool](root, "handoff-parent")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := testdbos.GetResult(handle, dbos.WithHandleTimeout(15*time.Second))
	switch testCase {
	case "full", "completed-history-missing":
		if err != nil || !ok {
			t.Fatal(ok, err)
		}
	case "history-missing":
		if err == nil || !strings.Contains(err.Error(), "event not found") {
			t.Fatal("history silently reselected", err)
		}
	case "boundary-missing":
		if err == nil || !strings.Contains(err.Error(), "retained boundary") {
			t.Fatal("boundary silently reselected", err)
		}
	case "grown":
		if err == nil || !strings.Contains(err.Error(), "TADW tooling exceeds budget") {
			t.Fatal("window silently rolled forward", err)
		}
	}
	a.mu.Lock()
	aCount := len(a.histories)
	a.mu.Unlock()
	b.mu.Lock()
	bCount := len(b.histories)
	b.mu.Unlock()
	if aCount != 0 || bCount != 0 {
		t.Fatal("request cache not released")
	}
}

func TestTADWHandoffChildCrashRecovery(t *testing.T) {
	for _, testCase := range []string{"full", "history-missing", "boundary-missing", "grown", "completed-history-missing"} {
		t.Run(testCase, func(t *testing.T) {
			store, _, cfg, request, _ := tadwToolingDB(t)
			selected := tadwProviderDatabaseAgent(t, store, request)
			tadwCacheTestTable(t, store)
			if _, err := store.Exec(`CREATE TABLE gatehouse_handoff_calls(id TEXT PRIMARY KEY,calls INTEGER NOT NULL)`); err != nil {
				t.Fatal(err)
			}
			ref := func(id int) model.SessionEventRef { return tadwToolingThinking(request, id) }
			for _, row := range []struct {
				id, parent    int
				kind, payload string
			}{
				{70, 10, "tool.request", `{"name":"lisp","code":"fourth","reason":"inspect"}`},
				{71, 70, "tool.success", `{"output":"` + strings.Repeat("z", 300) + `"}`},
				{80, 10, "thinking.request", `{}`},
				{85, 10, "tool.request", `{"name":"lisp","code":"fifth","reason":"inspect"}`},
				{86, 85, "tool.success", `{"output":"later"}`},
				{90, 10, "thinking.request", `{}`},
			} {
				if _, err := store.Exec(`INSERT INTO gatehouse_session_events(workspace,session,id,parent,kind,author_agent,payload,created_at) VALUES('w','s',?,?,?,?,?,?)`, ref(row.id).Id, ref(row.parent).Id, row.kind, selected.Ref.Id, row.payload, strconv.Itoa(row.id)); err != nil {
					t.Fatal(err)
				}
			}
			for index, attempt := range []int{11, 40, 60, 80, 90} {
				var previous *model.SessionEventRef
				if index > 0 {
					value := ref([]int{13, 41, 61, 81}[index-1])
					previous = &value
				}
				payload, err := database.SessionEventPayloadFrom(model.ThinkingRequestPayload{Turn: index + 1, Previous: previous})
				if err != nil {
					t.Fatal(err)
				}
				raw, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload=? WHERE id=?`, string(raw), ref(attempt).Id); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload='{"text":"TADW_PRIVATE_PAYLOAD"}' WHERE id=?`, ref(1).Id); err != nil {
				t.Fatal(err)
			}
			history, err := loadTADWHistory(context.Background(), store, ref(5), nil, cfg)
			if err != nil {
				t.Fatal(err)
			}
			all, err := loadTADWContext(context.Background(), store, history, request, selected.Ref, ref(40), cfg)
			if err != nil {
				t.Fatal(err)
			}
			// Force two rollovers, including one in the child interrupted by the crash.
			history, cfg = tadwTideBudget(history, all.ToolingBytes()-1)
			cfg.HistoryMinBytes = cfg.HistoryMaxBytes
			expected := tadwHandoffTestExpectation{Limits: cfg, IDs: history.Window.EventIDs}
			from := ""
			rollovers := 0
			for _, attempt := range []int{11, 40, 60, 80, 90} {
				window, err := loadTADWToolingWindow(context.Background(), store, history, request, selected.Ref, ref(attempt), cfg, from)
				if err != nil {
					t.Fatal(err)
				}
				if from != window.FromEventID {
					rollovers++
				}
				from = window.FromEventID
				expected.Attempts = append(expected.Attempts, tadwTideExpectation{From: from, XML: window.Transcript.String()})
			}
			if rollovers < 2 {
				t.Fatal("fixture did not roll twice", expected.Attempts)
			}
			var seq int
			var dbName, path string
			if err := store.QueryRow(`PRAGMA database_list`).Scan(&seq, &dbName, &path); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(expected)
			if err := os.WriteFile(path+".handoff", raw, 0600); err != nil {
				t.Fatal(err)
			}
			run := func(phase string) {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTADWHandoffProcess$", "-test.timeout=25s")
				cmd.Env = append(os.Environ(), "DBOS__APPVERSION=", "DBOS__VMID=tadw-handoff-"+phase, "GATEHOUSE_TEST_TADW_HANDOFF_DB="+path, "GATEHOUSE_TEST_TADW_HANDOFF_PHASE="+phase, "GATEHOUSE_TEST_TADW_HANDOFF_CASE="+testCase)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%s: %v\n%s", phase, err, out)
				}
			}
			run("fresh")
			assertTADWHandoffStorage(t, store, expected, 4)
			var status string
			if err := store.QueryRow(`SELECT status FROM workflow_status WHERE workflow_uuid='handoff-round:80'`).Scan(&status); err != nil || status != "PENDING" {
				t.Fatal("not a real child crash", status, err)
			}
			if _, err := store.Exec(`DROP TRIGGER handoff_history_probe; DROP TRIGGER handoff_tooling_probe; DROP TABLE gatehouse_context_cache_hints; DROP INDEX gatehouse_session_events_thread_order; DROP INDEX gatehouse_session_events_agent_terminals;
    UPDATE gatehouse_session_files SET name='résumé<&.txt';
    UPDATE gatehouse_session_events SET payload='invalid JSON' WHERE id=?`, ref(20).Id); err != nil {
				t.Fatal(err)
			}
			switch testCase {
			case "history-missing", "completed-history-missing":
				if _, err := store.Exec(`DELETE FROM gatehouse_session_events WHERE id=?`, history.Window.EventIDs[0]); err != nil {
					t.Fatal(err)
				}
			case "boundary-missing":
				if _, err := store.Exec(`DELETE FROM gatehouse_session_events WHERE id=?`, expected.Attempts[3].From); err != nil {
					t.Fatal(err)
				}
			case "grown":
				payload, _ := json.Marshal(map[string]string{"output": strings.Repeat("q", int(cfg.ContextMaxBytes))})
				if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload=? WHERE id=?`, string(payload), expected.Attempts[3].From); err != nil {
					t.Fatal(err)
				}
			}
			run("recover")
			var executor string
			if err := store.QueryRow(`SELECT executor_id FROM workflow_status WHERE workflow_uuid='handoff-round:80'`).Scan(&executor); err != nil || executor != "tadw-handoff-recover" {
				t.Fatal("child did not move to another executor", executor, err)
			}
			rounds := 4
			if testCase == "full" {
				rounds = 5
			}
			assertTADWHandoffStorage(t, store, expected, rounds)
			// The interrupted child's provider step was already complete. Recovery must
			// not call it again. Completed earlier children must not prepare again either.
			for _, attempt := range []int{11, 40, 60, 80, 90} {
				want := 1
				if attempt == 11 {
					want = 2
				}
				if attempt == 90 && testCase != "full" {
					want = 0
				}
				if attempt == 80 && testCase != "full" && testCase != "completed-history-missing" {
					want = 0
				}
				var calls int
				if err := store.QueryRow(`SELECT COALESCE((SELECT calls FROM gatehouse_handoff_calls WHERE id=?),0)`, ref(attempt).Id).Scan(&calls); err != nil || calls != want {
					t.Fatal(attempt, calls, want, err)
				}
			}
		})
	}
}

func assertTADWHandoffStorage(t *testing.T, store *database.Store, expected tadwHandoffTestExpectation, rounds int) {
	t.Helper()
	rows, err := store.Query(`SELECT function_name,output FROM operation_outputs`)
	if err != nil {
		t.Fatal(err)
	}
	historyCount, toolingCount, listCopies := 0, 0, 0
	for rows.Next() {
		var name string
		var output *string
		if err := rows.Scan(&name, &output); err != nil {
			t.Fatal(err)
		}
		if name == "DBOS.getWorkflowSteps" {
			t.Fatal("reader checkpointed listing data")
		}
		if name == tadwHistoryCheckpointName {
			historyCount++
		}
		if name == tadwToolingCheckpointName {
			toolingCount++
		}
		if output != nil {
			decoded, err := base64.StdEncoding.DecodeString(*output)
			if err != nil {
				continue
			}
			var ids []string
			if json.Unmarshal(decoded, &ids) == nil && reflect.DeepEqual(ids, expected.IDs) {
				listCopies++
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if historyCount != 1 || toolingCount != rounds || listCopies != 1 {
		t.Fatal("nonminimal durable state", historyCount, toolingCount, listCopies, rounds)
	}
	for index, attempt := range []int{11, 40, 60, 80, 90} {
		if index >= rounds {
			break
		}
		var output string
		if err := store.QueryRow(`SELECT output FROM operation_outputs WHERE workflow_uuid=? AND function_name=?`, "handoff-round:"+strconv.Itoa(attempt), tadwToolingCheckpointName).Scan(&output); err != nil {
			t.Fatal(err)
		}
		decoded, err := base64.StdEncoding.DecodeString(output)
		if err != nil {
			t.Fatal(err)
		}
		var boundary string
		if err := json.Unmarshal(decoded, &boundary); err != nil || boundary != expected.Attempts[index].From {
			t.Fatal("wrong durable boundary", attempt, boundary, err)
		}
	}
	assertTADWDBOSPayloadAbsent(t, store)
	var leaks int
	if err := store.QueryRow(`SELECT count(*) FROM workflow_status WHERE inputs LIKE '%TADW_PRIVATE%' OR output LIKE '%TADW_PRIVATE%'`).Scan(&leaks); err != nil || leaks != 0 {
		t.Fatal(leaks, err)
	}
}

func TestTADWHandoffReferenceValidationAndMemo(t *testing.T) {
	t.Setenv("DBOS__APPVERSION", "")
	t.Setenv("DBOS__VMID", "tadw-handoff-unit")
	store, _, cfg, request, _ := tadwToolingDB(t)
	selected := tadwProviderDatabaseAgent(t, store, request)
	tadwCacheTestTable(t, store)
	root := tadwHandoffRoot(t, store)
	worker := tadwHandoffTestWorker(t, root, store)
	owner := func(ctx dbos.Context, request model.SessionEventRef) (tadwHistoryHandoff, error) {
		if _, err := newTADWHandoffWorker(ctx, store); err == nil {
			return tadwHistoryHandoff{}, errors.New("accepted workflow-bound reader")
		}
		return worker.Prepare(ctx, request, tadwToolingThinking(request, 5), selected, false, cfg)
	}
	dbos.RegisterWorkflow(root, owner, dbos.WithWorkflowName("test.handoff-unit"))
	if err := testdbos.Launch(root); err != nil {
		t.Fatal(err)
	}
	handle, err := dbos.RunWorkflow(root, owner, request, dbos.WithWorkflowID(tadwHistoryOwnerWorkflowID(request)))
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := testdbos.GetResult(handle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handoff.compatible(selected, false, cfg); err != nil {
		t.Fatal(err)
	}
	drift := *selected
	drift.Model += "-changed"
	if _, err := handoff.compatible(&drift, false, cfg); err == nil {
		t.Fatal("handoff accepted selected configuration drift")
	}
	changedLimits := cfg
	changedLimits.ContextMaxBytes++
	if _, err := handoff.compatible(selected, false, changedLimits); err == nil {
		t.Fatal("handoff accepted limits drift")
	}
	original, err := worker.load(context.Background(), handoff, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Concurrent callers receive independent slice ownership and shared immutable
	// strings. No failed/cancelled caller can poison a later cold load.
	worker.Release(handoff)
	var group sync.WaitGroup
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			history, err := worker.load(context.Background(), handoff, cfg)
			if err != nil {
				t.Error(err)
				return
			}
			if !reflect.DeepEqual(history.History, original.History) {
				t.Error("changed history")
			}
			history.History[0].XML = "changed"
			history.Window.EventIDs[0] = "changed"
		}()
	}
	group.Wait()
	got, err := worker.load(context.Background(), handoff, cfg)
	if err != nil || !reflect.DeepEqual(got.History, original.History) {
		t.Fatal("memo aliases caller", err)
	}
	worker.Release(handoff)
	// Hold the application's sole connection so a real SDK read blocks. Child
	// cancellation must interrupt it even though no workflow values are copied.
	connection, err := store.DB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	timed, cancelTimed := context.WithTimeout(context.Background(), 20*time.Millisecond)
	_, readErr := worker.output(timed, handoff.Checkpoint, tadwHistoryCheckpointName)
	cancelTimed()
	connection.Close()
	if !errors.Is(readErr, context.DeadlineExceeded) {
		t.Fatal("reader ignored child deadline", readErr)
	}
	// A cancelled cold-reader leader must not cancel a healthy child sharing
	// that request's history. Keep the connection held until its leader exits.
	connection, err = store.DB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderDone, followerDone := make(chan error, 1), make(chan error, 1)
	go func() { _, err := worker.load(leaderCtx, handoff, cfg); leaderDone <- err }()
	until := time.Now().Add(time.Second)
	for {
		worker.mu.Lock()
		pending := len(worker.histories) > 0
		worker.mu.Unlock()
		if pending {
			break
		}
		if time.Now().After(until) {
			connection.Close()
			cancelLeader()
			t.Fatal("cold reader did not start")
		}
		time.Sleep(time.Millisecond)
	}
	go func() { _, err := worker.load(context.Background(), handoff, cfg); followerDone <- err }()
	time.Sleep(5 * time.Millisecond)
	cancelLeader()
	leaderErr := <-leaderDone
	connection.Close()
	if !errors.Is(leaderErr, context.Canceled) {
		t.Fatal(leaderErr)
	}
	if err := <-followerDone; err != nil {
		t.Fatal("leader cancellation poisoned healthy child", err)
	}
	worker.Release(handoff)
	readContext, closeReader := worker.reader(context.Background())
	if _, err := dbos.GetWorkflowID(readContext); err == nil {
		t.Fatal("reader inherited workflow state")
	}
	closeReader()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := worker.load(canceled, handoff, cfg); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// Lookup limits one output and validates ID/name/error, never falls back to a
	// different row or to history selection.
	for _, change := range []func(*tadwHistoryHandoff){
		func(h *tadwHistoryHandoff) { h.Request.Session.Id = "other" },
		func(h *tadwHistoryHandoff) { h.Checkpoint.WorkflowID = "missing" },
		func(h *tadwHistoryHandoff) { h.Checkpoint.StepID = -1 },
		func(h *tadwHistoryHandoff) { h.Checkpoint.StepID = 123 },
	} {
		bad := handoff
		change(&bad)
		if _, err := worker.load(context.Background(), bad, cfg); err == nil {
			t.Fatal("accepted bad locator", bad)
		}
	}
	// A store change is visible on cold restore, never by reselecting membership.
	if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload='{"text":"changed after checkpoint"}' WHERE id=?`, original.Window.EventIDs[0]); err != nil {
		t.Fatal(err)
	}
	got, err = worker.load(context.Background(), handoff, cfg)
	if err != nil || reflect.DeepEqual(got.History, original.History) {
		t.Fatal("cold load did not rehydrate", err)
	}
	worker.Release(handoff)
	if _, err := store.Exec(`DELETE FROM gatehouse_session_events WHERE id=?`, original.Window.EventIDs[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.load(context.Background(), handoff, cfg); err == nil {
		t.Fatal("missing history silently reselected")
	}
	if len(worker.histories) != 0 {
		t.Fatal("failed load retained poisoned entry")
	}
}
