// Package testdbos provides short polling intervals for workflow test fixtures.
package testdbos

import (
	"fmt"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

const PollInterval = 10 * time.Millisecond

// Launch configures every database-backed queue visible to this test application
// before starting workers. This also covers custom queues and restarted workers.
func Launch(ctx dbos.Context) error {
	queues, err := dbos.ListQueues(ctx)
	if err != nil {
		return fmt.Errorf("list test workflow queues: %w", err)
	}
	for _, queue := range queues {
		if err := queue.SetPollingInterval(ctx, PollInterval); err != nil {
			return fmt.Errorf("configure test workflow queue %q: %w", queue.GetName(), err)
		}
	}
	return dbos.Launch(ctx)
}

// GetResult shortens polling for retrieved and queued workflow handles while
// preserving caller-supplied timeouts and error handling.
func GetResult[R any](handle dbos.WorkflowHandle[R], options ...dbos.GetResultOption) (R, error) {
	options = append([]dbos.GetResultOption{dbos.WithHandlePollingInterval(PollInterval)}, options...)
	return handle.GetResult(options...)
}
