package agent

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"gatehouse/internal/testdbos"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

func TestTADWHandoffEmptyCheckpointAndDensePrefix(t *testing.T) {
	t.Setenv("DBOS__APPVERSION", "")
	t.Setenv("DBOS__VMID", "tadw-handoff-empty")
	store, _, limits, request, _ := tadwToolingDB(t)
	selected := tadwProviderDatabaseAgent(t, store, request)
	tadwCacheTestTable(t, store)
	limits.HistoryMaxBytes, limits.HistoryMinBytes = 1, 1
	root := tadwHandoffRoot(t, store)
	worker := tadwHandoffTestWorker(t, root, store)
	owner := func(ctx dbos.Context, request model.SessionEventRef) (tadwHistoryHandoff, error) {
		return worker.Prepare(ctx, request, tadwToolingThinking(request, 5), selected, false, limits)
	}
	gap := func(ctx dbos.Context, _ bool) (bool, error) {
		if _, err := dbos.RunAsStep(ctx, func(context.Context) (bool, error) { return true, nil }, dbos.WithStepName("test.gap-first")); err != nil {
			return false, err
		}
		if _, err := dbos.RunAsStep(ctx, func(context.Context) (bool, error) { return true, nil }, dbos.WithStepName("test.gap-target")); err != nil {
			return false, err
		}
		// Disposable corruption control: OFFSET 1 is not function ID 1 after a hole.
		if _, err := store.ExecContext(ctx, `DELETE FROM operation_outputs WHERE workflow_uuid='handoff-gap' AND function_id=0`); err != nil {
			return false, err
		}
		_, err := worker.capture(ctx, "test.gap-target")
		return err != nil && strings.Contains(err.Error(), "dense committed"), nil
	}
	dbos.RegisterWorkflow(root, owner, dbos.WithWorkflowName("test.handoff-empty"))
	dbos.RegisterWorkflow(root, gap, dbos.WithWorkflowName("test.handoff-gap"))
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
	worker.Release(handoff)
	if _, err := store.Exec(`DROP TABLE gatehouse_context_cache_hints; DROP INDEX gatehouse_session_events_thread_order; DROP INDEX gatehouse_session_events_agent_terminals`); err != nil {
		t.Fatal(err)
	}
	history, err := worker.load(context.Background(), handoff, limits)
	if err != nil || len(history.History) != 0 || len(history.Window.EventIDs) != 0 || history.Window.ToolingBudgetBytes != limits.ContextMaxBytes {
		t.Fatal("empty checkpoint was reselected", history, err)
	}
	worker.Release(handoff)
	var original string
	if err := store.QueryRow(`SELECT output FROM operation_outputs WHERE workflow_uuid=? AND function_id=0`, handoff.Checkpoint.WorkflowID).Scan(&original); err != nil {
		t.Fatal(err)
	}
	for _, update := range []string{
		`function_name='wrong-name'`,
		`error='{"message":"recorded failure"}'`,
		`output='` + base64.StdEncoding.EncodeToString([]byte(`{"not":"an ID list"}`)) + `'`,
	} {
		if _, err := store.Exec(`UPDATE operation_outputs SET `+update+` WHERE workflow_uuid=? AND function_id=0`, handoff.Checkpoint.WorkflowID); err != nil {
			t.Fatal(err)
		}
		if _, err := worker.load(context.Background(), handoff, limits); err == nil {
			t.Fatal("accepted damaged checkpoint", update)
		}
		if _, err := store.Exec(`UPDATE operation_outputs SET function_name=?,error=NULL,output=? WHERE workflow_uuid=? AND function_id=0`, tadwHistoryCheckpointName, original, handoff.Checkpoint.WorkflowID); err != nil {
			t.Fatal(err)
		}
	}
	gapHandle, err := dbos.RunWorkflow(root, gap, true, dbos.WithWorkflowID("handoff-gap"))
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := testdbos.GetResult(gapHandle); err != nil || !ok {
		t.Fatal("sparse prefix accepted", ok, err)
	}
	if _, err := worker.output(context.Background(), tadwCheckpointRef{WorkflowID: "handoff-gap", StepID: 1}, "test.gap-target"); err == nil {
		t.Fatal("row offset treated as function ID")
	}
}

func TestTADWHandoffWorkerMemoIsBounded(t *testing.T) {
	store, _, _, _, _ := tadwToolingDB(t)
	root := tadwHandoffRoot(t, store)
	worker := tadwHandoffTestWorker(t, root, store)
	// Only map/list mechanics are probed here; these synthetic entries never touch
	// DBOS or pretend to be recoverable checkpoints.
	var keys []tadwHistoryMemoKey
	worker.mu.Lock()
	for i := 0; i < tadwHandoffWorkerHistoryEntries; i++ {
		key := tadwHistoryMemoKey{History: tadwHistoryHandoff{Checkpoint: tadwCheckpointRef{WorkflowID: fmt.Sprint(i)}}}
		entry := &tadwHistoryMemoEntry{done: make(chan struct{})}
		close(entry.done)
		keys = append(keys, key)
		worker.remember(key, entry)
	}
	worker.order.MoveToFront(worker.histories[keys[0]].position)
	extra := tadwHistoryMemoKey{History: tadwHistoryHandoff{Checkpoint: tadwCheckpointRef{WorkflowID: "extra"}}}
	worker.remember(extra, &tadwHistoryMemoEntry{done: make(chan struct{})})
	if len(worker.histories) != tadwHandoffWorkerHistoryEntries || worker.order.Len() != len(worker.histories) || worker.histories[keys[0]] == nil || worker.histories[keys[1]] != nil {
		t.Fatal("LRU is not bounded or evicted the hot entry")
	}
	worker.mu.Unlock()
	for _, key := range keys {
		worker.Release(key.History)
	}
	worker.Release(extra.History)
	if len(worker.histories) != 0 || worker.order.Len() != 0 {
		t.Fatal("release left cache entries")
	}
}
