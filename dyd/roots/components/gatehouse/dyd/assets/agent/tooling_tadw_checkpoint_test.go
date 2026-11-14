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
	"strconv"
	"strings"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/internal/testdbos"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

type tadwTideExpectation struct {
	From string
	XML  string
}

func TestTADWToolingCheckpointProcess(t *testing.T) {
	path := os.Getenv("GATEHOUSE_TEST_TADW_TIDE_DB")
	if path == "" {
		t.Skip("subprocess helper")
	}
	phase, testCase := os.Getenv("GATEHOUSE_TEST_TADW_TIDE_PHASE"), os.Getenv("GATEHOUSE_TEST_TADW_TIDE_CASE")
	budget, err := strconv.ParseInt(os.Getenv("GATEHOUSE_TEST_TADW_TIDE_BUDGET"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path + ".expected")
	if err != nil {
		t.Fatal(err)
	}
	var expected []tadwTideExpectation
	if err := json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	err, store := database.Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	workflowContext, err := dbos.NewContext(context.Background(), dbos.Config{AppName: "tadw-tide-test", ApplicationVersion: "tadw-tide-v1", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(workflowContext, 5*time.Second)
	if phase == "fresh" {
		// If the helper rehydrates after checkpointing rather than reusing the
		// fresh buffer, the returned file reference will differ from expected.
		if _, err := store.Exec(`CREATE TRIGGER tide_after_checkpoint AFTER INSERT ON operation_outputs
		 WHEN NEW.function_name='gatehouse.tadw-tooling-window'
		 BEGIN UPDATE gatehouse_session_files SET name='TADW_POST_WINDOW_NAME'; END`); err != nil {
			t.Fatal(err)
		}
	}
	workflow := func(ctx dbos.Context, request model.SessionEventRef) (bool, error) {
		history, err := checkpointTADWHistory(ctx, store, tadwToolingThinking(request, 5), nil, tadwAdapterConfig(10000))
		if err != nil {
			return false, err
		}
		history, cfg := tadwTideBudget(history, budget)
		from := ""
		for index, attempt := range []int{11, 40, 60, 80, 90} {
			window, err := checkpointTADWTooling(ctx, store, history, request, model.WorkspaceAgentRef{Workspace: request.Session.Workspace, Id: "agent"}, tadwToolingThinking(request, attempt), cfg, from)
			if err != nil {
				return false, err
			}
			if window.FromEventID != expected[index].From || window.Transcript.String() != expected[index].XML {
				return false, fmt.Errorf("attempt %d did not reconstruct its recorded window", attempt)
			}
			from = window.FromEventID
			if phase == "fresh" {
				var name string
				if err := store.QueryRowContext(ctx, `SELECT name FROM gatehouse_session_files`).Scan(&name); err != nil || name != "TADW_POST_WINDOW_NAME" {
					return false, fmt.Errorf("fresh buffer probe did not fire: %q (%v)", name, err)
				}
				if _, err := store.ExecContext(ctx, `UPDATE gatehouse_session_files SET name='résumé<&.txt'`); err != nil {
					return false, err
				}
			}
			if _, err := dbos.RunAsStep(ctx, func(context.Context) (bool, error) { return true, nil }, dbos.WithStepName("test.tide-thinking")); err != nil {
				return false, err
			}
			if phase == "fresh" && attempt == 80 {
				os.Exit(0)
			}
		}
		return true, nil
	}
	dbos.RegisterWorkflow(workflowContext, workflow, dbos.WithWorkflowName("test.tadw-tide"))
	if err := testdbos.Launch(workflowContext); err != nil {
		t.Fatal(err)
	}
	if phase == "fresh" {
		request := model.SessionEventRef{Session: model.SessionRef{Workspace: model.WorkspaceRef{Id: "w"}, Id: "s"}, Id: fmt.Sprintf("sev_%026d", 10)}
		handle, err := dbos.RunWorkflow(workflowContext, workflow, request, dbos.WithWorkflowID("tadw-tide-request"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testdbos.GetResult(handle, dbos.WithHandleTimeout(15*time.Second)); err != nil {
			t.Fatal(err)
		}
		t.Fatal("fresh worker did not crash")
	}
	handle, err := dbos.RetrieveWorkflow[bool](workflowContext, "tadw-tide-request")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := testdbos.GetResult(handle, dbos.WithHandleTimeout(15*time.Second))
	switch testCase {
	case "full":
		if err != nil || !ok {
			t.Fatal(ok, err)
		}
	case "missing":
		if err == nil || !strings.Contains(err.Error(), "retained boundary") {
			t.Fatal("missing recorded base silently replaced", err)
		}
	case "grown":
		if err == nil || !strings.Contains(err.Error(), "TADW tooling exceeds budget") {
			t.Fatal("replay reselected instead of enforcing recorded window", err)
		}
	default:
		t.Fatal(testCase)
	}
}

func TestTADWToolingCheckpointCrashRecovery(t *testing.T) {
	for _, testCase := range []string{"full", "missing", "grown"} {
		t.Run(testCase, func(t *testing.T) {
			store, initial, cfg, request, agent := tadwToolingDB(t)
			ref := func(id int) model.SessionEventRef { return tadwToolingThinking(request, id) }
			// Add enough execution for a second rollover and a final fitting round.
			for _, row := range []struct {
				id, parent int
				kind, at   string
				payload    any
			}{
				{70, 10, "tool.request", "70", map[string]any{"name": "lisp", "code": "fourth", "reason": "inspect"}},
				{71, 70, "tool.success", "71", map[string]any{"output": strings.Repeat("z", 300)}},
				{80, 10, "thinking.request", "80", map[string]any{}},
				{85, 10, "tool.request", "85", map[string]any{"name": "lisp", "code": "fifth", "reason": "inspect"}},
				{86, 85, "tool.success", "86", map[string]any{"output": "later"}},
				{90, 10, "thinking.request", "90", map[string]any{}},
			} {
				raw, err := json.Marshal(row.payload)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.Exec(`INSERT INTO gatehouse_session_events(workspace,session,id,parent,kind,author_agent,payload,created_at) VALUES('w','s',?,?,?,'agent',?,?)`, ref(row.id).Id, ref(row.parent).Id, row.kind, string(raw), row.at); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload='{"text":"TADW_PRIVATE_PAYLOAD"}' WHERE id=?`, ref(1).Id); err != nil {
				t.Fatal(err)
			}
			var err error
			initial, err = loadTADWHistory(context.Background(), store, ref(5), nil, cfg)
			if err != nil {
				t.Fatal(err)
			}
			all, err := loadTADWContext(context.Background(), store, initial, request, agent, ref(40), cfg)
			if err != nil {
				t.Fatal(err)
			}
			budget := all.ToolingBytes() - 1
			initial, cfg = tadwTideBudget(initial, budget)
			from := ""
			var expected []tadwTideExpectation
			rollovers := 0
			for _, attempt := range []int{11, 40, 60, 80, 90} {
				window, err := loadTADWToolingWindow(context.Background(), store, initial, request, agent, ref(attempt), cfg, from)
				if err != nil {
					t.Fatal(err)
				}
				if window.FromEventID != from {
					rollovers++
				}
				from = window.FromEventID
				expected = append(expected, tadwTideExpectation{From: from, XML: window.Transcript.String()})
			}
			if rollovers < 2 || expected[1].From == expected[3].From {
				t.Fatal("fixture did not roll twice", expected)
			}
			var sequence int
			var name, path string
			if err := store.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(expected)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path+".expected", raw, 0600); err != nil {
				t.Fatal(err)
			}
			run := func(phase string) {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTADWToolingCheckpointProcess$", "-test.timeout=25s")
				cmd.Env = append(os.Environ(), "DBOS__APPVERSION=", "DBOS__VMID=tadw-tide-test-worker", "GATEHOUSE_TEST_TADW_TIDE_DB="+path, "GATEHOUSE_TEST_TADW_TIDE_PHASE="+phase, "GATEHOUSE_TEST_TADW_TIDE_CASE="+testCase, "GATEHOUSE_TEST_TADW_TIDE_BUDGET="+strconv.FormatInt(budget, 10))
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%s worker: %v\n%s", phase, err, out)
				}
			}
			run("fresh")
			assertTADWTideStorage(t, store, initial.Window.EventIDs, expected[:4])
			var status string
			if err := store.QueryRow(`SELECT status FROM workflow_status WHERE workflow_uuid='tadw-tide-request'`).Scan(&status); err != nil || status != "PENDING" {
				t.Fatal("not a pending crash", status, err)
			}
			if _, err := store.Exec(`DROP TRIGGER tide_after_checkpoint; UPDATE gatehouse_session_events SET payload='invalid JSON' WHERE id=?`, ref(20).Id); err != nil {
				t.Fatal(err)
			}
			switch testCase {
			case "missing":
				if _, err := store.Exec(`DELETE FROM gatehouse_session_events WHERE id=?`, expected[1].From); err != nil {
					t.Fatal(err)
				}
			case "grown":
				// This older retained result could be dropped by a fresh rollover,
				// but replay must fail, not replace the already recorded boundary.
				payload, err := json.Marshal(map[string]any{"output": strings.Repeat("q", int(budget)-200)})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload=? WHERE id=?`, string(payload), expected[1].From); err != nil {
					t.Fatal(err)
				}
				if _, err := loadTADWToolingWindow(context.Background(), store, initial, request, agent, ref(40), cfg, expected[1].From); err != nil {
					t.Fatal("growth case cannot distinguish selection from replay", err)
				}
			}
			run("recover")
			count := 4
			if testCase == "full" {
				count = 5
			}
			assertTADWTideStorage(t, store, initial.Window.EventIDs, expected[:count])
			assertTADWDBOSPayloadAbsent(t, store)
		})
	}
}

func assertTADWTideStorage(t *testing.T, store *database.Store, historyIDs []string, expected []tadwTideExpectation) {
	t.Helper()
	rows, err := store.Query(`SELECT function_name,output FROM operation_outputs WHERE workflow_uuid='tadw-tide-request' ORDER BY function_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var boundaries []string
	histories, thinking := 0, 0
	for rows.Next() {
		var name, output string
		if err := rows.Scan(&name, &output); err != nil {
			t.Fatal(err)
		}
		raw, err := base64.StdEncoding.DecodeString(output)
		if err != nil {
			t.Fatal(err)
		}
		switch name {
		case tadwHistoryCheckpointName:
			histories++
			var ids []string
			if err := json.Unmarshal(raw, &ids); err != nil || !slices.Equal(ids, historyIDs) {
				t.Fatal("history checkpoint copied/changed", string(raw), err)
			}
		case tadwToolingCheckpointName:
			var from string
			if err := json.Unmarshal(raw, &from); err != nil {
				t.Fatal("tooling checkpoint is not a boundary string", string(raw), err)
			}
			boundaries = append(boundaries, from)
		case "test.tide-thinking":
			thinking++
			if string(raw) != "true" {
				t.Fatal("thinking step stored context", string(raw))
			}
		default:
			t.Fatal("unexpected operation", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, window := range expected {
		want = append(want, window.From)
	}
	if histories != 1 || thinking != len(expected) || !reflect.DeepEqual(boundaries, want) {
		t.Fatal(histories, thinking, boundaries, want)
	}
}
