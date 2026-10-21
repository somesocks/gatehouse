package agent

import (
	"context"
	"fmt"
	"strings"

	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

// CreateMessage commits the message, recipient snapshots, agent requests, and
// durable workflow enqueues together. DBOS cannot execute uncommitted work.
func (runtime *SessionEventReplyRuntime) CreateMessage(ctx context.Context, event model.SessionEvent) (error, model.SessionEvent) {
	transaction, err := runtime.store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session message: %w", err), model.SessionEvent{}
	}
	defer transaction.Rollback()
	err, events := runtime.store.SessionMessagesCreateInTransaction(ctx, transaction, event)
	if err != nil {
		return err, model.SessionEvent{}
	}
	durable := dbos.From(runtime.dbos, ctx)
	for _, request := range events[1:] {
		_, err := dbos.Enqueue[model.SessionEventRef](durable, sessionEventReplyQueue, "gatehouse.session-event-reply", SessionEventReplyInput{Event: request.Ref},
			dbos.WithEnqueueWorkflowID(sessionEventReplyWorkflowID(request.Ref)),
			dbos.WithEnqueueQueuePartitionKey(sessionEventReplyTaskPartition(request.Ref)),
			dbos.WithEnqueueClassName("SessionEventReplyRuntime"), dbos.WithEnqueueConfigName(runtime.ConfigName()),
			dbos.WithEnqueueTransaction(transaction))
		if err != nil {
			return fmt.Errorf("enqueue session reply %q: %w", request.Ref.Id, err), model.SessionEvent{}
		}
	}
	if text, _ := event.Payload["text"].(string); len(events) > 1 && strings.TrimSpace(text) != "" {
		err, name := runtime.store.SessionNameGetInTransaction(ctx, transaction, event.Ref.Session)
		if err != nil {
			return err, model.SessionEvent{}
		}
		if name == nil {
			_, err := dbos.Enqueue[bool](durable, sessionNameQueue, "gatehouse.session-name", SessionNameInput{Session: event.Ref.Session},
				dbos.WithEnqueueWorkflowID(sessionNameWorkflowID(event.Ref.Session)),
				dbos.WithEnqueueQueuePartitionKey(sessionEventReplyPartition(event.Ref.Session)),
				dbos.WithEnqueueClassName("SessionEventReplyRuntime"), dbos.WithEnqueueConfigName(runtime.ConfigName()),
				dbos.WithEnqueueTransaction(transaction))
			if err != nil {
				return fmt.Errorf("enqueue session name: %w", err), model.SessionEvent{}
			}
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session message: %w", err), model.SessionEvent{}
	}
	return nil, events[0]
}

// RespondToApproval persists the terminal response and delivers its reference to
// the waiting workflow in the same transaction, just like RespondToInput.
func (runtime *SessionEventReplyRuntime) RespondToApproval(ctx context.Context, event model.SessionEvent) (error, model.SessionEvent) {
	transaction, err := runtime.store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin approval response: %w", err), model.SessionEvent{}
	}
	defer transaction.Rollback()
	err, stored := runtime.store.SessionApprovalResponseCreateInTransaction(ctx, transaction, event)
	if err != nil {
		return err, model.SessionEvent{}
	}
	if err := dbos.Send(dbos.From(runtime.dbos, ctx), sessionApprovalWorkflowID(*stored.Parent), stored.Ref, "response", dbos.WithSendTransaction(transaction)); err != nil {
		return fmt.Errorf("send approval response %q: %w", stored.Ref.Id, err), model.SessionEvent{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit approval response: %w", err), model.SessionEvent{}
	}
	return nil, stored
}
