package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	"gatehouse/database"
	"gatehouse/model"
)

const sessionEventReplyQueue = "gatehouse.session-event-replies"

type SessionEventReplyInput struct {
	Event model.SessionEventRef
}

type SessionEventReplyRuntime struct {
	store *database.Store
	dbos  dbos.Context
	queue dbos.Queue
}

func NewSessionEventReplyRuntime(ctx dbos.Context, store *database.Store) (error, *SessionEventReplyRuntime) {
	queue, err := dbos.RegisterQueue(ctx, sessionEventReplyQueue,
		dbos.WithPartitionQueue(),
		dbos.WithGlobalConcurrency(1),
	)
	if err != nil {
		return fmt.Errorf("register session event reply queue: %w", err), nil
	}
	runtime := &SessionEventReplyRuntime{store: store, dbos: ctx, queue: queue}
	dbos.RegisterWorkflow(ctx, runtime.reply,
		dbos.WithInstance(runtime),
		dbos.WithWorkflowName("gatehouse.session-event-reply"),
	)
	return nil, runtime
}

func (runtime *SessionEventReplyRuntime) ConfigName() string {
	return "gatehouse"
}

func (runtime *SessionEventReplyRuntime) Reconcile() error {
	err, tasks := runtime.store.SessionEventReplyTasksGet(runtime.dbos, 100)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		_, err := dbos.RunWorkflow(runtime.dbos, runtime.reply, SessionEventReplyInput{Event: task.Event},
			dbos.WithRunInstance(runtime),
			dbos.WithWorkflowID(sessionEventReplyWorkflowID(task.Event)),
			dbos.WithQueue(runtime.queue),
			dbos.WithQueuePartitionKey(sessionEventReplyPartition(task.Event.Session)),
		)
		if err != nil {
			return fmt.Errorf("enqueue session event reply %q: %w", task.Event.Id, err)
		}
		if err := runtime.store.SessionEventReplyTaskDelete(runtime.dbos, task.Event); err != nil {
			return err
		}
	}
	return nil
}

func (runtime *SessionEventReplyRuntime) reply(ctx dbos.Context, input SessionEventReplyInput) (model.SessionEvent, error) {
	return dbos.RunAsStep(ctx, func(step context.Context) (model.SessionEvent, error) {
		replyRef := model.SessionEventRef{Session: input.Event.Session, Id: sessionEventReplyID(input.Event)}
		err, existing := runtime.store.SessionEventGet(step, replyRef)
		if err != nil {
			return model.SessionEvent{}, err
		}
		if existing != nil {
			return *existing, nil
		}
		err, selected := runtime.store.WorkspaceAgentModelSelect(step, input.Event.Session.Workspace)
		if err != nil {
			return model.SessionEvent{}, err
		}
		if selected == nil {
			return model.SessionEvent{}, fmt.Errorf("reply to session event %q: no enabled workspace agent", input.Event.Id)
		}
		if selected.Protocol != "builtin" {
			return model.SessionEvent{}, fmt.Errorf("reply to session event %q: unsupported provider protocol %q", input.Event.Id, selected.Protocol)
		}
		err, text := BuiltinReply(selected.Model, selected.Parameters)
		if err != nil {
			return model.SessionEvent{}, err
		}
		event := model.SessionEvent{
			Ref:         replyRef,
			Parent:      &input.Event,
			Kind:        "message.text",
			AuthorAgent: &selected.Ref,
			Payload:     map[string]interface{}{"text": text},
		}
		err, stored := runtime.store.SessionEventsCreate(step, event)
		if err != nil {
			return model.SessionEvent{}, err
		}
		return stored, nil
	}, dbos.WithStepName("gatehouse.session-event-reply"))
}

func sessionEventReplyWorkflowID(event model.SessionEventRef) string {
	return "session-event-reply:" + event.Id
}

func sessionEventReplyPartition(session model.SessionRef) string {
	return session.Workspace.Id + "/" + session.Id
}

func sessionEventReplyID(event model.SessionEventRef) string {
	sum := sha256.Sum256([]byte(event.Session.Workspace.Id + "\x00" + event.Session.Id + "\x00" + event.Id))
	value := sum[:16]
	value[6] = value[6]&0x0f | 0x50
	value[8] = value[8]&0x3f | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}
