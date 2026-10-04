package agent

import (
	"context"
	"fmt"

	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

// RespondToInput commits the terminal event, draft deletion, and durable DBOS
// message together. Only the event reference is sent to the waiting workflow.
func (runtime *SessionEventReplyRuntime) RespondToInput(ctx context.Context, event model.SessionEvent) (error, model.SessionEvent) {
	transaction, err := runtime.store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session input response: %w", err), model.SessionEvent{}
	}
	defer transaction.Rollback()
	err, stored := runtime.store.SessionInputResponseCreateInTransaction(ctx, transaction, event)
	if err != nil {
		return err, model.SessionEvent{}
	}
	err, tool := runtime.store.SessionInputRequestToolGetInTransaction(ctx, transaction, *stored.Parent)
	if err != nil {
		return err, model.SessionEvent{}
	}
	if err := dbos.Send(runtime.dbos, sessionToolCallWorkflowID(tool), stored.Ref, sessionInputResponseTopic(*stored.Parent), dbos.WithSendTransaction(transaction)); err != nil {
		return fmt.Errorf("send input response %q: %w", stored.Ref.Id, err), model.SessionEvent{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session input response: %w", err), model.SessionEvent{}
	}
	return nil, stored
}
