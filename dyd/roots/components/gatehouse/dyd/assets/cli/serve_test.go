package cli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
	_ "github.com/dbos-inc/dbos-transact-golang/dbos/driver/sqlite"

	"gatehouse/config"
	"gatehouse/database"
)

func TestServeDBOSRecoversWaitingInputAfterShutdown(t *testing.T) {
	t.Setenv("DBOS__APPVERSION", "")
	t.Setenv("DBOS__VMID", "gatehouse-restart-test")
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "gatehouse.sqlite")}
	serveContext, stopServe := context.WithCancel(context.Background())
	defer stopServe()

	err, store := database.Open(serveContext, configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	first, err := newServeDBOSContext(serveContext, configuration, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := dbos.Shutdown(first, 5*time.Second); err != nil {
			t.Errorf("stop first DBOS context: %v", err)
		}
	})
	if first.GetApplicationVersion() != serveDBOSApplicationVersion {
		t.Fatalf("application version = %q, want %q", first.GetApplicationVersion(), serveDBOSApplicationVersion)
	}

	entered := make(chan struct{}, 1)
	wait := func(ctx dbos.Context, _ struct{}) (string, error) {
		entered <- struct{}{}
		return dbos.Recv[string](ctx, "response", time.Hour)
	}
	dbos.RegisterWorkflow(first, wait, dbos.WithWorkflowName("test.serve-input-wait"))
	if err := dbos.Launch(first); err != nil {
		t.Fatal(err)
	}
	const workflowID = "session-input:restart-test"
	if _, err := dbos.RunWorkflow(first, wait, struct{}{}, dbos.WithWorkflowID(workflowID)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("input workflow did not start waiting")
	}

	stopServe()
	if err := first.Err(); err != nil {
		t.Fatalf("server signal cancelled DBOS before Shutdown: %v", err)
	}
	if err := dbos.Shutdown(first, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	err, reopened := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var status string
	if err := reopened.QueryRowContext(context.Background(), `SELECT status FROM workflow_status WHERE workflow_uuid = ?`, workflowID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(dbos.WorkflowStatusPending) {
		t.Fatalf("input waiter after shutdown = %q, want PENDING", status)
	}

	second, err := newServeDBOSContext(context.Background(), configuration, reopened)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := dbos.Shutdown(second, 5*time.Second); err != nil {
			t.Errorf("stop recovered DBOS context: %v", err)
		}
	})
	dbos.RegisterWorkflow(second, wait, dbos.WithWorkflowName("test.serve-input-wait"))
	if err := dbos.Launch(second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("input workflow was not recovered on restart")
	}
	if err := dbos.Send(second, workflowID, "Ada", "response"); err != nil {
		t.Fatal(err)
	}
	handle, err := dbos.RetrieveWorkflow[string](second, workflowID)
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		result string
		err    error
	}
	completed := make(chan outcome, 1)
	go func() {
		result, err := handle.GetResult()
		completed <- outcome{result, err}
	}()
	select {
	case got := <-completed:
		if got.err != nil || got.result != "Ada" {
			t.Fatalf("recovered input result = (%q, %v), want Ada", got.result, got.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("recovered input did not receive its response")
	}
}
