package testdbos_test

import (
	"context"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/internal/testdbos"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	_ "github.com/dbos-inc/dbos-transact-golang/dbos/driver/sqlite"
)

func TestLaunchConfiguresApplicationQueuesAndPreservesOtherSettings(t *testing.T) {
	err, store := database.Open(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, err := dbos.NewContext(context.Background(), dbos.Config{AppName: "short-polling-test", ApplicationVersion: "test-v1", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(ctx, time.Second)
	other, err := dbos.NewContext(context.Background(), dbos.Config{AppName: "other-polling-test", ApplicationVersion: "test-v1", SQLiteSystemDB: store.DB})
	if err != nil {
		t.Fatal(err)
	}
	defer dbos.Shutdown(other, time.Second)
	otherQueue, err := dbos.RegisterQueue(other, "other-app-queue")
	if err != nil {
		t.Fatal(err)
	}
	otherInterval := otherQueue.GetPollingInterval()
	first, err := dbos.RegisterQueue(ctx, "first", dbos.WithGlobalConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbos.RegisterQueue(ctx, "second", dbos.WithPartitionQueue()); err != nil {
		t.Fatal(err)
	}
	work := func(_ dbos.Context, value int) (int, error) { return value, nil }
	dbos.RegisterWorkflow(ctx, work, dbos.WithWorkflowName("test.short-polling"))
	if err := testdbos.Launch(ctx); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first", "second"} {
		queue, err := dbos.RetrieveQueue(ctx, name)
		if err != nil || queue.GetPollingInterval() != testdbos.PollInterval {
			t.Fatalf("queue %q polling configuration: %#v, %v", name, queue, err)
		}
		if name == "first" && (queue.GetGlobalConcurrency() == nil || *queue.GetGlobalConcurrency() != 1) {
			t.Fatal("polling setup changed queue concurrency")
		}
		if name == "second" && !queue.GetPartitionQueue() {
			t.Fatal("polling setup changed queue partitioning")
		}
	}
	unchanged, err := dbos.RetrieveQueue(other, "other-app-queue")
	if err != nil || unchanged.GetPollingInterval() != otherInterval {
		t.Fatalf("polling setup changed another application's queue: %#v, %v", unchanged, err)
	}
	handle, err := dbos.RunWorkflow(ctx, work, 42, dbos.WithWorkflowID("short-polling-result"), dbos.WithQueue(first))
	if err != nil {
		t.Fatal(err)
	}
	// A retrieved handle reads completion from the database, exercising the
	// polling path rather than the in-process workflow result channel.
	handle, err = dbos.RetrieveWorkflow[int](ctx, handle.GetWorkflowID())
	if err != nil {
		t.Fatal(err)
	}
	got, err := testdbos.GetResult(handle, dbos.WithHandleTimeout(5*time.Second))
	if err != nil || got != 42 {
		t.Fatalf("workflow result = %d, %v; want 42", got, err)
	}
}
