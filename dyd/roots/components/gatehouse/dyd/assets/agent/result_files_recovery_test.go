package agent_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gatehouse/agent"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/model"
	"gatehouse/storage"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"modernc.org/sqlite"
)

func TestResultPublicationCrashProcess(t *testing.T) {
	path := os.Getenv("GATEHOUSE_RESULT_RECOVERY_PATH")
	if path == "" {
		t.Skip("subprocess helper")
	}
	crash := os.Getenv("GATEHOUSE_RESULT_RECOVERY_CRASH")
	if crash != "" {
		if err := sqlite.RegisterScalarFunction("gh_result_publication_crash", 0, func(_ *sqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
			os.Exit(73)
			return nil, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err, ring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer ring.Close()
	var source model.SessionEventRef
	if err := json.Unmarshal([]byte(os.Getenv("GATEHOUSE_RESULT_RECOVERY_SOURCE")), &source); err != nil {
		t.Fatal(err)
	}
	workflowContext, err := dbos.NewContext(ctx, dbos.Config{AppName: "result-files-test", ApplicationVersion: "results-v1", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	err, runtime := agent.NewSessionEventReplyRuntime(workflowContext, store, ring)
	if err != nil {
		t.Fatal(err)
	}
	work := agent.ResultFileTestPublication(runtime, os.Getenv("GATEHOUSE_RESULT_RECOVERY_OUTCOME"), os.Getenv("GATEHOUSE_RESULT_RECOVERY_BODY"))
	dbos.RegisterWorkflow(workflowContext, work, dbos.WithWorkflowName("test.result-publication-crash"))
	if crash != "" {
		trigger := `CREATE TRIGGER crash_result_publication BEFORE INSERT ON operation_outputs
			WHEN NEW.function_name IN ('gatehouse.session-event-agent-persist', 'gatehouse.agent-provider-decision')
			BEGIN SELECT gh_result_publication_crash(); END`
		if crash == "after-commit" {
			trigger = `CREATE TRIGGER crash_result_publication BEFORE UPDATE ON workflow_status
				WHEN NEW.name = 'test.result-publication-crash' AND NEW.status = 'SUCCESS'
				BEGIN SELECT gh_result_publication_crash(); END`
		}
		if _, err := store.Exec(trigger); err != nil {
			t.Fatal(err)
		}
	}
	if err := dbos.Launch(workflowContext); err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(workflowContext, time.Second)
	workflowID := "result-publication-crash:" + source.Id
	if crash != "" {
		handle, err := dbos.RunWorkflow(workflowContext, work, source, dbos.WithWorkflowID(workflowID))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := handle.GetResult(); err != nil {
			t.Fatal(err)
		}
		t.Fatal("expected process termination at publication boundary")
	}
	handle, err := dbos.RetrieveWorkflow[model.SessionEventRef](workflowContext, workflowID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.GetResult(); err != nil {
		t.Fatal(err)
	}
}

func TestResultPublicationRecovery(t *testing.T) {
	const firstMarker = "PUBLICATION_FIRST_BODY_3da2"
	const secondMarker = "PUBLICATION_SECOND_BODY_9e17"
	smallA, smallB := firstMarker, secondMarker
	largeA, largeB := strings.Repeat(firstMarker, 300), strings.Repeat(secondMarker, 400)
	for _, test := range []struct {
		name, requestKind, firstOutcome, firstBody, nextOutcome, nextBody, crash string
	}{
		{"tool-small-to-large", "tool.request", "success", smallA, "success", largeB, "before-commit"},
		{"tool-large-to-small-failure", "tool.request", "success", largeA, "failure", smallB, "before-commit"},
		{"tool-large-to-different-large", "tool.request", "success", largeA, "success", largeB, "before-commit"},
		{"provider-small-to-large", "thinking.request", "failure", smallA, "failure", largeB, "before-commit"},
		{"provider-large-to-different-large", "thinking.request", "failure", largeA, "failure", largeB, "before-commit"},
		{"provider-success-to-large-failure", "thinking.request", "success", smallA, "failure", largeB, "before-commit"},
		{"committed-tool-with-storage-failure", "tool.request", "success", largeA, "failure", largeB, "after-commit"},
		{"committed-provider-with-storage-failure", "thinking.request", "failure", largeA, "failure", largeB, "after-commit"},
		{"committed-retry-with-storage-failure", "thinking.request", "retry", "", "failure", largeB, "after-commit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "publication.sqlite")
			f := newResultFileFixtureWithDatabase(t, 4096, "", config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: path})
			request := f.request(t, test.requestKind, `"unused"`)
			source, err := json.Marshal(request.Ref)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.store.Close(); err != nil {
				t.Fatal(err)
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			run := func(outcome, body, crash string) {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				process := exec.CommandContext(ctx, executable, "-test.run=^TestResultPublicationCrashProcess$", "-test.v")
				process.Env = append(os.Environ(), "GATEHOUSE_RESULT_RECOVERY_PATH="+path, "GATEHOUSE_RESULT_RECOVERY_SOURCE="+string(source), "GATEHOUSE_RESULT_RECOVERY_OUTCOME="+outcome, "GATEHOUSE_RESULT_RECOVERY_BODY="+body, "GATEHOUSE_RESULT_RECOVERY_CRASH="+crash)
				output, err := process.CombinedOutput()
				if crash != "" {
					if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 73 {
						t.Fatalf("crash helper: %v\n%s", err, output)
					}
				} else if err != nil {
					t.Fatalf("recovery helper: %v\n%s", err, output)
				}
			}
			run(test.firstOutcome, test.firstBody, test.crash)
			err, store := database.Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: path})
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			var before int
			if err := store.QueryRow(`SELECT count(*) FROM gatehouse_session_events WHERE parent = ?`, request.Ref.Id).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if test.crash == "before-commit" && before != 0 {
				t.Fatal("event escaped the rolled-back transaction")
			}
			if test.crash == "after-commit" && before == 0 {
				t.Fatal("event did not commit before workflow completion")
			}
			if _, err := store.Exec(`DROP TRIGGER crash_result_publication`); err != nil {
				t.Fatal(err)
			}
			if test.crash == "after-commit" {
				if _, err := store.Exec(`CREATE TRIGGER reject_replay_publication BEFORE INSERT ON gatehouse_storage_objects BEGIN SELECT RAISE(ABORT, 'replay storage rejected'); END`); err != nil {
					t.Fatal(err)
				}
			}
			run(test.nextOutcome, test.nextBody, "")
			wantOutcome, wantBody := test.nextOutcome, test.nextBody
			if test.crash == "after-commit" {
				wantOutcome, wantBody = test.firstOutcome, test.firstBody
			}
			wantKind := model.SessionEventKindToolSuccess
			if test.requestKind == "thinking.request" {
				wantKind = model.SessionEventKindThinkingSuccess
				if wantOutcome == "failure" {
					wantKind = model.SessionEventKindThinkingFailure
				}
				if wantOutcome == "retry" {
					wantKind = model.SessionEventKindThinkingUpdate
				}
			} else if wantOutcome == "failure" {
				wantKind = model.SessionEventKindToolFailure
			}
			err, event := store.SessionEventChildGet(context.Background(), request.Ref, wantKind)
			if err != nil || event == nil {
				t.Fatalf("missing %s outcome: %v", wantKind, err)
			}
			if wantOutcome == "retry" {
				if event.Payload["reason"] != "rate_limit" {
					t.Fatal("provider retry contract changed")
				}
			} else {
				body, _ := event.Payload["output"].(string)
				if test.requestKind == "thinking.request" {
					body, _ = event.Payload["message"].(string)
				}
				fileRef, present, err := database.SessionEventOutputFile(*event)
				if err != nil || present != (len(wantBody) > 4096) {
					t.Fatalf("wrong placement: %#v %v", event.Payload, err)
				}
				if present {
					err, file, _ := store.SessionFileGet(context.Background(), fileRef, f.principal)
					if err != nil || file == nil {
						t.Fatalf("missing output file: %v", err)
					}
					err, reader := storage.NewClient(store, nil).Get(context.Background(), file.StorageObject.Id, 0)
					if err != nil {
						t.Fatal(err)
					}
					contents, err := io.ReadAll(reader)
					reader.Close()
					if err != nil {
						t.Fatal(err)
					}
					body = string(contents)
				}
				if body != wantBody {
					t.Fatal("event and body describe different attempts")
				}
			}
			var children, placementSteps int
			if err := store.QueryRow(`SELECT count(*) FROM gatehouse_session_events WHERE parent = ?`, request.Ref.Id).Scan(&children); err != nil || children != 1 {
				t.Fatalf("expected one committed outcome: %d %v", children, err)
			}
			if err := store.QueryRow(`SELECT count(*) FROM operation_outputs WHERE function_name = 'gatehouse.agent-result-store'`).Scan(&placementSteps); err != nil || placementSteps != 0 {
				t.Fatal("placement was checkpointed independently")
			}
			assertDBOSDoesNotContain(t, store, firstMarker)
			assertDBOSDoesNotContain(t, store, secondMarker)
		})
	}
}
