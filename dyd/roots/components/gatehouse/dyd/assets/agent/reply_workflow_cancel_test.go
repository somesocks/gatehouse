package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
	_ "github.com/dbos-inc/dbos-transact-golang/dbos/driver/sqlite"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

func TestCancelReplyWorkflowTree(t *testing.T) {
	t.Setenv("DBOS__APPVERSION", "")
	t.Setenv("DBOS__VMID", "gatehouse-tool-cancel-test")
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "gatehouse.sqlite")}
	ctx := context.Background()
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	dbosContext, err := dbos.NewContext(ctx, dbos.Config{
		AppName: "gatehouse-tool-cancel-test", ApplicationVersion: "gatehouse-tool-cancel-test-v1", SQLiteSystemDB: store.DB,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbos.Shutdown(dbosContext, 5*time.Second) })

	reply := model.SessionEventRef{Id: "sev_00000000000000000000000000"}
	tools := []model.SessionEventRef{
		{Id: "sev_00000000000000000000000001"},
		{Id: "sev_00000000000000000000000002"},
		{Id: "sev_00000000000000000000000003"},
	}
	approval := sessionApprovalWorkflowID(model.SessionEventRef{Id: "sev_00000000000000000000000005"})
	queue, err := dbos.RegisterQueue(dbosContext, "gatehouse-tool-cancel-test-tools", dbos.WithGlobalConcurrency(2))
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan int, 2)
	queuedRan := make(chan struct{}, 1)
	wait := func(workflowContext dbos.Context, _ int) (bool, error) {
		return dbos.Recv[bool](workflowContext, "response", time.Hour)
	}
	tool := func(workflowContext dbos.Context, index int) (bool, error) {
		if index == 2 {
			queuedRan <- struct{}{}
			return true, nil
		}
		if index == 0 {
			started <- index
			return dbos.Recv[bool](workflowContext, "input:sev_00000000000000000000000004", time.Hour)
		}
		child, err := dbos.RunWorkflow(workflowContext, wait, index, dbos.WithWorkflowID(approval))
		if err != nil {
			return false, err
		}
		started <- index
		return child.GetResult()
	}
	parent := func(workflowContext dbos.Context, _ struct{}) (bool, error) {
		var first dbos.WorkflowHandle[bool]
		for index, request := range tools[:2] {
			handle, err := dbos.RunWorkflow(workflowContext, tool, index,
				dbos.WithWorkflowID(sessionToolCallWorkflowID(request)), dbos.WithQueue(queue))
			if err != nil {
				return false, err
			}
			if first == nil {
				first = handle
			}
		}
		<-started
		<-started
		if _, err := dbos.RunWorkflow(workflowContext, tool, 2,
			dbos.WithWorkflowID(sessionToolCallWorkflowID(tools[2])), dbos.WithQueue(queue)); err != nil {
			return false, err
		}
		return first.GetResult()
	}
	register := func(workflowContext dbos.Context) {
		dbos.RegisterWorkflow(workflowContext, wait, dbos.WithWorkflowName("test.reply-cancel-input-wait"))
		dbos.RegisterWorkflow(workflowContext, tool, dbos.WithWorkflowName("test.reply-cancel-tool"))
		dbos.RegisterWorkflow(workflowContext, parent, dbos.WithWorkflowName("test.reply-cancel-parent"))
	}
	register(dbosContext)
	if err := dbos.Launch(dbosContext); err != nil {
		t.Fatal(err)
	}
	if _, err := dbos.RunWorkflow(dbosContext, parent, struct{}{}, dbos.WithWorkflowID(sessionEventReplyWorkflowID(reply))); err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{
		sessionEventReplyWorkflowID(reply):  "PENDING",
		sessionToolCallWorkflowID(tools[0]): "PENDING",
		sessionToolCallWorkflowID(tools[1]): "PENDING",
		sessionToolCallWorkflowID(tools[2]): "ENQUEUED",
		approval:                            "PENDING",
	}
	checkWorkflowStatuses(t, store, statuses, 10*time.Second)

	runtime := &SessionEventReplyRuntime{dbos: dbosContext}
	if err := runtime.cancelReplyWorkflowTree(dbosContext, reply); err != nil {
		t.Fatal(err)
	}
	for id := range statuses {
		statuses[id] = "CANCELLED"
	}
	checkWorkflowStatuses(t, store, statuses, 10*time.Second)
	if err := runtime.cancelReplyWorkflowTree(dbosContext, reply); err != nil {
		t.Fatalf("repeated cancellation: %v", err)
	}
	if err := dbos.Shutdown(dbosContext, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	err, reopened := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restarted, err := dbos.NewContext(ctx, dbos.Config{
		AppName: "gatehouse-tool-cancel-test", ApplicationVersion: "gatehouse-tool-cancel-test-v1", SQLiteSystemDB: reopened.DB,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbos.Shutdown(restarted, 5*time.Second) })
	if _, err := dbos.RegisterQueue(restarted, "gatehouse-tool-cancel-test-tools", dbos.WithGlobalConcurrency(2)); err != nil {
		t.Fatal(err)
	}
	register(restarted)
	if err := dbos.Launch(restarted); err != nil {
		t.Fatal(err)
	}
	checkWorkflowStatuses(t, reopened, statuses, 10*time.Second)
	select {
	case <-queuedRan:
		t.Fatal("queued tool ran after reply cancellation")
	default:
	}
}

func checkWorkflowStatuses(t *testing.T, store *database.Store, statuses map[string]string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		matches := true
		for id, want := range statuses {
			var got string
			err := store.QueryRowContext(context.Background(), `SELECT status FROM workflow_status WHERE workflow_uuid = ?`, id).Scan(&got)
			if err != nil || got != want {
				matches = false
				if time.Now().After(deadline) {
					t.Fatalf("workflow %q status = (%q, %v), want %q", id, got, err, want)
				}
				break
			}
		}
		if matches {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}
