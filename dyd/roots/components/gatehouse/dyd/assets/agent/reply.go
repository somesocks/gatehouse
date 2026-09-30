package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	"gatehouse/authz"
	"gatehouse/database"
	"gatehouse/diagnostics"
	"gatehouse/inputform"
	"gatehouse/keychain"
	"gatehouse/lisp"
	"gatehouse/model"
	"gatehouse/sessionsearch"
	"gatehouse/storage"
	"gatehouse/typed_id"
)

func init() {
	diagnostics.Register("agent.tool_call.evaluate")
}

const (
	sessionEventReplyQueue = "gatehouse.session-event-replies"
	sessionToolCallQueue   = "gatehouse.session-tool-calls"
	sessionNameQueue       = "gatehouse.session-names"
	sessionNamePrompt      = "Generate a concise title for this conversation. Return only the title, using a few words."
	sessionApprovalWait    = time.Duration(1<<63 - 1)
	lispCancellationPoll   = time.Second
	replyCancellationGrace = time.Second
)

type SessionEventReplyInput struct {
	Event model.SessionEventRef
}

type SessionReplyCancellationInput struct {
	Request model.SessionEvent
}

type SessionNameInput struct {
	Session model.SessionRef
}

type SessionToolCallInput struct {
	Request model.SessionEventRef
}

type SessionApprovalInput struct {
	Request model.SessionEventRef
}

type SessionInputWaitInput struct {
	Request model.SessionEventRef
}

type InputOutcome struct {
	Result  string
	Failure string
}

type ApprovalOutcome string

const (
	ApprovalGranted  ApprovalOutcome = "approved"
	ApprovalRejected ApprovalOutcome = "rejected"
)

type sessionToolCallExecution struct {
	Kind   string
	Code   string
	Output string
}

type agentFinalReply struct {
	Text        string
	Attachments []string
}

type agentProviderTurn struct {
	Final   *agentFinalReply
	Calls   []openAICompatibleToolCall
	Metrics *model.SessionEventMetrics
}

type sessionNamePreparation struct {
	Selected *database.WorkspaceAgentModel
	Text     string
}

type sessionReplyCancelled struct{}

func (sessionReplyCancelled) Error() string { return "reply cancelled" }

type SessionEventReplyRuntime struct {
	store      *database.Store
	keyring    *keychain.Keyring
	storage    *storage.Client
	dbos       dbos.Context
	dataSource *dbos.DataSource
	queue      dbos.Queue
	toolQueue  dbos.Queue
	nameQueue  dbos.Queue
}

func NewSessionEventReplyRuntime(ctx dbos.Context, store *database.Store, keyring *keychain.Keyring) (error, *SessionEventReplyRuntime) {
	queue, err := dbos.RegisterQueue(ctx, sessionEventReplyQueue, dbos.WithPartitionQueue())
	if err != nil {
		return fmt.Errorf("register session event reply queue: %w", err), nil
	}
	toolQueue, err := dbos.RegisterQueue(ctx, sessionToolCallQueue, dbos.WithGlobalConcurrency(10))
	if err != nil {
		return fmt.Errorf("register session tool-call queue: %w", err), nil
	}
	nameQueue, err := dbos.RegisterQueue(ctx, sessionNameQueue, dbos.WithPartitionQueue(), dbos.WithGlobalConcurrency(1))
	if err != nil {
		return fmt.Errorf("register session name queue: %w", err), nil
	}
	var dataSource *dbos.DataSource
	if pool := store.PostgresPool(); pool != nil {
		dataSource, err = dbos.NewDataSource(ctx, pool, dbos.WithDataSourceName("gatehouse"))
	} else {
		dataSource, err = dbos.NewDataSource(ctx, store.DB, dbos.WithDataSourceName("gatehouse"))
	}
	if err != nil {
		return fmt.Errorf("register Gatehouse database data source: %w", err), nil
	}
	runtime := &SessionEventReplyRuntime{store: store, keyring: keyring, storage: storage.NewClient(store, keyring), dbos: ctx, dataSource: dataSource, queue: queue, toolQueue: toolQueue, nameQueue: nameQueue}
	dbos.RegisterWorkflow(ctx, runtime.reply,
		dbos.WithInstance(runtime),
		dbos.WithWorkflowName("gatehouse.session-event-reply"),
	)
	dbos.RegisterWorkflow(ctx, runtime.cancelReply,
		dbos.WithInstance(runtime),
		dbos.WithWorkflowName("gatehouse.session-reply-cancel"),
	)
	dbos.RegisterWorkflow(ctx, runtime.toolCall,
		dbos.WithInstance(runtime),
		dbos.WithWorkflowName("gatehouse.session-tool-call"),
	)
	dbos.RegisterWorkflow(ctx, runtime.approval,
		dbos.WithInstance(runtime),
		dbos.WithWorkflowName("gatehouse.session-approval"),
	)
	dbos.RegisterWorkflow(ctx, runtime.input,
		dbos.WithInstance(runtime),
		dbos.WithWorkflowName("gatehouse.session-input"),
	)
	dbos.RegisterWorkflow(ctx, runtime.nameSession,
		dbos.WithInstance(runtime),
		dbos.WithWorkflowName("gatehouse.session-name"),
	)
	return nil, runtime
}

func (runtime *SessionEventReplyRuntime) ConfigName() string {
	return "gatehouse"
}

// CancelReply starts one recoverable cancellation per agent request. The HTTP
// caller waits only for the request event, not the grace period or tool calls.
func (runtime *SessionEventReplyRuntime) CancelReply(ctx context.Context, request model.SessionEvent) (error, model.SessionEvent) {
	if request.Kind != model.SessionEventKindCancelRequest || request.Parent == nil || request.AuthorPrincipal == nil {
		return fmt.Errorf("cancel reply: invalid request"), model.SessionEvent{}
	}
	id := sessionReplyCancellationWorkflowID(*request.Parent)
	// Repeated requests must retrieve the existing cancellation even after its
	// target reply has entered a terminal state.
	_, err := dbos.RetrieveWorkflow[model.SessionEventRef](runtime.dbos, id)
	if errors.Is(err, dbos.ErrNonExistentWorkflow) {
		err, reply := runtime.store.SessionEventChildGet(ctx, *request.Parent, model.SessionEventKindAgentSuccess)
		if err != nil {
			return err, model.SessionEvent{}
		}
		if reply != nil {
			return database.ErrSessionReplyAlreadyCompleted, model.SessionEvent{}
		}
		handle, err := dbos.RetrieveWorkflow[model.SessionEventRef](runtime.dbos, sessionEventReplyWorkflowID(*request.Parent))
		if err == nil {
			status, err := handle.GetStatus()
			if err != nil {
				return err, model.SessionEvent{}
			}
			switch status.Status {
			case dbos.WorkflowStatusSuccess, dbos.WorkflowStatusError, dbos.WorkflowStatusCancelled, dbos.WorkflowStatusMaxRecoveryAttemptsExceeded:
				return database.ErrSessionReplyAlreadyCompleted, model.SessionEvent{}
			}
		} else if !errors.Is(err, dbos.ErrNonExistentWorkflow) {
			return err, model.SessionEvent{}
		}
	} else if err != nil {
		return err, model.SessionEvent{}
	}
	_, err = dbos.RunWorkflow(runtime.dbos, runtime.cancelReply, SessionReplyCancellationInput{Request: request},
		dbos.WithRunInstance(runtime), dbos.WithWorkflowID(id))
	if err != nil {
		return fmt.Errorf("start session reply cancellation %q: %w", request.Parent.Id, err), model.SessionEvent{}
	}
	reference, err := dbos.GetEvent[model.SessionEventRef](dbos.From(runtime.dbos, ctx), id, "request", 10*time.Second)
	if err != nil {
		return fmt.Errorf("await session reply cancellation request %q: %w", request.Parent.Id, err), model.SessionEvent{}
	}
	err, stored := runtime.store.SessionEventGet(ctx, reference)
	if err != nil {
		return err, model.SessionEvent{}
	}
	if stored == nil {
		return fmt.Errorf("session reply cancellation request %q is unavailable", reference.Id), model.SessionEvent{}
	}
	return nil, *stored
}

func (runtime *SessionEventReplyRuntime) cancelReply(ctx dbos.Context, input SessionReplyCancellationInput) (model.SessionEventRef, error) {
	request := input.Request
	if request.Kind != model.SessionEventKindCancelRequest || request.Parent == nil || request.AuthorPrincipal == nil || request.Ref.Session != request.Parent.Session {
		return model.SessionEventRef{}, fmt.Errorf("session reply cancellation request is invalid")
	}
	storedRequest, err := dbos.RunAsTransaction(ctx, runtime.dataSource, func(step context.Context, tx dbos.Tx) (model.SessionEvent, error) {
		err, stored := runtime.store.SessionEventCreateInTransaction(step, tx, request)
		return stored, err
	}, dbos.WithStepName("gatehouse.session-reply-cancel-request"))
	if err != nil {
		return model.SessionEventRef{}, err
	}
	request = storedRequest
	if err := dbos.SetEvent(ctx, "request", request.Ref); err != nil {
		return model.SessionEventRef{}, err
	}
	handle, err := dbos.RunWorkflow(ctx, runtime.reply, SessionEventReplyInput{Event: *request.Parent},
		dbos.WithRunInstance(runtime), dbos.WithWorkflowID(sessionEventReplyWorkflowID(*request.Parent)),
		dbos.WithQueue(runtime.queue), dbos.WithQueuePartitionKey(sessionEventReplyTaskPartition(*request.Parent)))
	if err != nil {
		return model.SessionEventRef{}, fmt.Errorf("start cancelled reply %q: %w", request.Parent.Id, err)
	}
	if err := runtime.cancelReplyWorkflowTree(ctx, *request.Parent); err != nil {
		return model.SessionEventRef{}, err
	}
	status, err := handle.GetStatus()
	if err != nil {
		return model.SessionEventRef{}, fmt.Errorf("read cancelled reply %q status: %w", request.Parent.Id, err)
	}
	kind := model.SessionEventKindCancelSuccess
	var payload map[string]interface{}
	switch status.Status {
	case dbos.WorkflowStatusCancelled:
	case dbos.WorkflowStatusSuccess:
		// A reply can observe cancel.request and finish cooperatively before
		// DBOS marks it CANCELLED. A real agent reply means it finished first.
		err, reply := runtime.store.SessionEventChildGet(ctx, *request.Parent, model.SessionEventKindAgentSuccess)
		if err != nil {
			return model.SessionEventRef{}, err
		}
		if reply != nil {
			kind = model.SessionEventKindCancelFailure
		}
	case dbos.WorkflowStatusError, dbos.WorkflowStatusMaxRecoveryAttemptsExceeded:
		kind = model.SessionEventKindCancelFailure
	default:
		return model.SessionEventRef{}, fmt.Errorf("reply %q has unexpected status %q after cancellation", request.Parent.Id, status.Status)
	}
	if kind == model.SessionEventKindCancelSuccess {
		if _, err := dbos.Sleep(ctx, replyCancellationGrace); err != nil {
			return model.SessionEventRef{}, fmt.Errorf("wait after cancelling reply %q: %w", request.Parent.Id, err)
		}
		payload, err = database.SessionEventPayloadFrom(model.CancelSuccessPayload{})
	} else {
		payload, err = database.SessionEventPayloadFrom(model.CancelFailurePayload{Code: "already_completed"})
	}
	if err != nil {
		return model.SessionEventRef{}, fmt.Errorf("encode cancellation outcome payload: %w", err)
	}
	agentFailureCode := "cancelled"
	if kind == model.SessionEventKindCancelFailure && (status.Status == dbos.WorkflowStatusError || status.Status == dbos.WorkflowStatusMaxRecoveryAttemptsExceeded) {
		agentFailureCode = "failed"
	}
	if err := runtime.cancelledReplyFinalize(ctx, request, agentFailureCode); err != nil {
		return model.SessionEventRef{}, err
	}
	err, reference := runtime.persistAgentEvent(ctx, model.SessionEvent{
		Ref: model.SessionEventRef{Session: request.Ref.Session}, Parent: &request.Ref, Kind: kind,
		AuthorPrincipal: request.AuthorPrincipal, Payload: payload, CreatedAt: eventTerminalCreatedAt(request.CreatedAt),
	})
	return reference, err
}

func (runtime *SessionEventReplyRuntime) cancelledReplyFinalize(ctx dbos.Context, cancellation model.SessionEvent, agentFailureCode string) error {
	request := *cancellation.Parent
	err, events := runtime.store.SessionEventsGet(ctx, request.Session)
	if err != nil {
		return fmt.Errorf("read cancelled reply event tree %q: %w", request.Id, err)
	}
	var agentRequest model.SessionEvent
	for _, event := range events {
		if event.Ref == request && event.Kind == model.SessionEventKindAgentRequest {
			agentRequest = event
			break
		}
	}
	if agentRequest.Ref.Id == "" {
		return fmt.Errorf("cancel agent request %q: request event is unavailable", request.Id)
	}
	children := make(map[string][]model.SessionEvent, len(events))
	for _, event := range events {
		if event.Parent != nil {
			children[event.Parent.Id] = append(children[event.Parent.Id], event)
		}
	}
	hasTerminal := func(parent model.SessionEvent, terminals ...string) bool {
		for _, child := range children[parent.Ref.Id] {
			for _, terminal := range terminals {
				if child.Kind == terminal {
					return true
				}
			}
		}
		return false
	}
	descendants := make([]model.SessionEvent, 0)
	queue := append([]model.SessionEvent(nil), children[request.Id]...)
	for len(queue) > 0 {
		event := queue[0]
		queue = queue[1:]
		descendants = append(descendants, event)
		queue = append(queue, children[event.Ref.Id]...)
	}
	// Close approval and input capabilities before finalizing their tools.
	for _, event := range descendants {
		switch event.Kind {
		case model.SessionEventKindInputRequest:
			if hasTerminal(event, model.SessionEventKindInputSuccess, model.SessionEventKindInputFailure) {
				continue
			}
			id, err := typed_id.New(typed_id.SessionEvent)
			if err != nil {
				return err
			}
			payload, err := database.SessionEventPayloadFrom(model.InputFailurePayload{Code: "cancelled"})
			if err != nil {
				return err
			}
			err, _ = runtime.store.SessionInputCancellationCreate(ctx, model.SessionEvent{
				Ref: model.SessionEventRef{Session: event.Ref.Session, Id: id}, Parent: &event.Ref,
				Kind: model.SessionEventKindInputFailure, AuthorPrincipal: cancellation.AuthorPrincipal, Payload: payload,
			})
			if errors.Is(err, database.ErrSessionInputResolved) {
				continue
			}
			if err != nil {
				return fmt.Errorf("cancel pending input %q: %w", event.Ref.Id, err)
			}
		case model.SessionEventKindApprovalRequest:
			if hasTerminal(event, model.SessionEventKindApprovalSuccess, model.SessionEventKindApprovalFailure) {
				continue
			}
			id, err := typed_id.New(typed_id.SessionEvent)
			if err != nil {
				return err
			}
			payload, err := database.SessionEventPayloadFrom(model.ApprovalFailurePayload{Code: "cancelled"})
			if err != nil {
				return err
			}
			err, _ = runtime.store.SessionApprovalCancellationCreate(ctx, model.SessionEvent{
				Ref: model.SessionEventRef{Session: event.Ref.Session, Id: id}, Parent: &event.Ref,
				Kind: model.SessionEventKindApprovalFailure, AuthorPrincipal: cancellation.AuthorPrincipal, Payload: payload,
			})
			if errors.Is(err, database.ErrSessionApprovalResolved) {
				continue
			}
			if err != nil {
				return fmt.Errorf("cancel pending approval %q: %w", event.Ref.Id, err)
			}
		}
	}
	for _, event := range descendants {
		switch event.Kind {
		case model.SessionEventKindToolRequest:
			if hasTerminal(event, model.SessionEventKindToolSuccess, model.SessionEventKindToolFailure) {
				continue
			}
			name, _ := event.Payload["name"].(string)
			callID, _ := event.Payload["call_id"].(string)
			if name == "" || callID == "" || event.AuthorAgent == nil {
				return fmt.Errorf("cancel unfinished tool %q: request payload is invalid", event.Ref.Id)
			}
			payload, err := database.SessionEventPayloadFrom(model.ToolFailurePayload{Name: name, CallId: callID, Code: "cancelled", Output: "execution cancelled"})
			if err != nil {
				return err
			}
			if err, _ := runtime.persistAgentEvent(ctx, model.SessionEvent{
				Ref: model.SessionEventRef{Session: event.Ref.Session}, Parent: &event.Ref,
				Kind: model.SessionEventKindToolFailure, AuthorAgent: event.AuthorAgent, Payload: payload,
				CreatedAt: eventTerminalCreatedAt(event.CreatedAt),
			}); err != nil {
				return fmt.Errorf("finish cancelled tool %q: %w", event.Ref.Id, err)
			}
		case model.SessionEventKindThinkingRequest:
			if hasTerminal(event, model.SessionEventKindThinkingSuccess, model.SessionEventKindThinkingFailure) {
				continue
			}
			if event.AuthorAgent == nil {
				return fmt.Errorf("cancel unfinished thinking %q: agent is unavailable", event.Ref.Id)
			}
			payload, err := database.SessionEventPayloadFrom(model.ThinkingFailurePayload{Code: "cancelled"})
			if err != nil {
				return err
			}
			if err, _ := runtime.persistAgentEvent(ctx, model.SessionEvent{
				Ref: model.SessionEventRef{Session: event.Ref.Session}, Parent: &event.Ref,
				Kind: model.SessionEventKindThinkingFailure, AuthorAgent: event.AuthorAgent, Payload: payload,
				CreatedAt: eventTerminalCreatedAt(event.CreatedAt),
			}); err != nil {
				return fmt.Errorf("finish cancelled thinking %q: %w", event.Ref.Id, err)
			}
		}
	}
	if !hasTerminal(agentRequest, model.SessionEventKindAgentSuccess, model.SessionEventKindAgentFailure) {
		agentID, _ := agentRequest.Payload["agent"].(string)
		if agentID == "" {
			return fmt.Errorf("cancel agent request %q: target agent is unavailable", request.Id)
		}
		payload, err := database.SessionEventPayloadFrom(model.AgentFailurePayload{Code: agentFailureCode})
		if err != nil {
			return err
		}
		if err, _ := runtime.persistAgentEvent(ctx, model.SessionEvent{
			Ref: model.SessionEventRef{Session: request.Session}, Parent: &request,
			Kind: model.SessionEventKindAgentFailure, AuthorAgent: &model.WorkspaceAgentRef{Workspace: request.Session.Workspace, Id: agentID}, Payload: payload,
			CreatedAt: eventTerminalCreatedAt(agentRequest.CreatedAt),
		}); err != nil {
			return fmt.Errorf("finish cancelled agent request %q: %w", request.Id, err)
		}
	}
	return nil
}

func (runtime *SessionEventReplyRuntime) Reconcile() error {
	err, tasks := runtime.store.SessionEventReplyTasksGet(runtime.dbos, 100)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		_, err = dbos.RunWorkflow(runtime.dbos, runtime.reply, SessionEventReplyInput{Event: task.Event},
			dbos.WithRunInstance(runtime),
			dbos.WithWorkflowID(sessionEventReplyWorkflowID(task.Event)),
			dbos.WithQueue(runtime.queue),
			dbos.WithQueuePartitionKey(sessionEventReplyTaskPartition(task.Event)),
		)
		if err != nil {
			return fmt.Errorf("enqueue session event reply %q: %w", task.Event.Id, err)
		}
		if err := runtime.store.SessionEventReplyTaskDelete(runtime.dbos, task.Event); err != nil {
			return err
		}
	}
	err, nameTasks := runtime.store.SessionNameTasksGet(runtime.dbos, 100)
	if err != nil {
		return err
	}
	for _, task := range nameTasks {
		_, err = dbos.RunWorkflow(runtime.dbos, runtime.nameSession, SessionNameInput{Session: task.Session},
			dbos.WithRunInstance(runtime),
			dbos.WithWorkflowID(sessionNameWorkflowID(task.Session)),
			dbos.WithQueue(runtime.nameQueue),
			dbos.WithQueuePartitionKey(sessionEventReplyPartition(task.Session)),
		)
		if err != nil {
			return fmt.Errorf("enqueue session name %q: %w", task.Session.Id, err)
		}
		if err := runtime.store.SessionNameTaskDelete(runtime.dbos, task.Session); err != nil {
			return err
		}
	}
	err, approvalTasks := runtime.store.SessionApprovalDecisionTasksGet(runtime.dbos, 100)
	if err != nil {
		return err
	}
	for _, task := range approvalTasks {
		err := dbos.Send(runtime.dbos, sessionApprovalWorkflowID(task.Approval), task.Response, "response", dbos.WithIdempotencyKey(task.Response.Id))
		if err != nil {
			return fmt.Errorf("send approval response %q: %w", task.Response.Id, err)
		}
		if err := runtime.store.SessionApprovalDecisionTaskDelivered(runtime.dbos, task.Approval); err != nil {
			return err
		}
	}
	err, inputTasks := runtime.store.SessionInputResponseTasksGet(runtime.dbos, 100)
	if err != nil {
		return err
	}
	for _, task := range inputTasks {
		if err := dbos.Send(runtime.dbos, sessionInputWorkflowID(task.Input), task.Response, "response", dbos.WithIdempotencyKey(task.Response.Id)); err != nil {
			return fmt.Errorf("send input response %q: %w", task.Response.Id, err)
		}
		if err := runtime.store.SessionInputResponseTaskDelivered(runtime.dbos, task.Input); err != nil {
			return err
		}
	}
	return nil
}

func (runtime *SessionEventReplyRuntime) nameSession(ctx dbos.Context, input SessionNameInput) (bool, error) {
	err, existing := runtime.store.SessionNameGet(ctx, input.Session)
	if err != nil {
		return false, err
	}
	if existing != nil {
		return true, nil
	}
	err, events := runtime.store.SessionEventsGet(ctx, input.Session)
	if err != nil {
		return false, err
	}
	preparation := sessionNamePreparation{}
	for _, event := range events {
		if event.Parent != nil || event.Kind != model.SessionEventKindMessageText || event.AuthorPrincipal == nil {
			continue
		}
		text, ok := event.Payload["text"].(string)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		values, ok := event.Payload["agents"].([]interface{})
		if !ok || len(values) == 0 {
			continue
		}
		preferred, _ := values[0].(string)
		if preferred == "" {
			continue
		}
		err, selected := runtime.store.WorkspaceAgentModelGet(ctx, input.Session.Workspace, preferred)
		if err != nil {
			return false, err
		}
		if selected == nil {
			return false, nil
		}
		preparation = sessionNamePreparation{Selected: selected, Text: text}
		break
	}
	if preparation.Selected == nil {
		return false, nil
	}
	err, name := runtime.sessionNameCompletion(ctx, preparation.Selected, preparation.Text)
	if err != nil {
		return false, err
	}
	err, updated := runtime.store.SessionNameSet(ctx, input.Session, name)
	return updated, err
}

func (runtime *SessionEventReplyRuntime) sessionNameCompletion(ctx dbos.Context, selected *database.WorkspaceAgentModel, text string) (error, string) {
	switch selected.Protocol {
	case "builtin":
		return BuiltinReply(selected.Model, selected.Parameters)
	case "openai-chat-completions":
		if selected.BaseURL == nil || selected.Keychain == nil || selected.APIKey == nil {
			return fmt.Errorf("name session with provider %q: missing credentials", selected.ProviderID), ""
		}
		completion, err := runtime.openAICompatibleComplete(ctx, selected, openAICompatibleRequest{
			Model:    selected.Model,
			Messages: []openAICompatibleMessage{{Role: "system", Content: sessionNamePrompt}, {Role: "user", Content: text}},
		})
		reply := completion.Message
		if err != nil || len(reply.ToolCalls) != 0 || strings.TrimSpace(reply.Content) == "" {
			if err == nil {
				err = fmt.Errorf("session name completion returned no title")
			}
			return err, ""
		}
		return nil, reply.Content
	case "openai-responses":
		if selected.BaseURL == nil || selected.Keychain == nil || selected.APIKey == nil {
			return fmt.Errorf("name session with provider %q: missing credentials", selected.ProviderID), ""
		}
		reply, err := runtime.openAIResponsesComplete(ctx, selected, openAIResponsesRequest{
			Model: selected.Model, Instructions: sessionNamePrompt, Input: []json.RawMessage{openAIResponsesMessage("user", text)},
		})
		if err != nil {
			return err, ""
		}
		name := ""
		for _, raw := range reply.Output {
			var output openAIResponsesOutput
			if err := json.Unmarshal(raw, &output); err != nil {
				return fmt.Errorf("decode session name response: %w", err), ""
			}
			if output.Type == "function_call" {
				return fmt.Errorf("session name completion attempted a tool call"), ""
			}
			if output.Type == "message" {
				for _, content := range output.Content {
					if content.Type == "output_text" {
						name += content.Text
					}
				}
			}
		}
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("session name completion returned no title"), ""
		}
		return nil, name
	default:
		return fmt.Errorf("unsupported provider protocol %q", selected.Protocol), ""
	}
}

func (runtime *SessionEventReplyRuntime) reply(ctx dbos.Context, input SessionEventReplyInput) (model.SessionEventRef, error) {
	reference, replyErr := runtime.replyAttempt(ctx, input)
	if replyErr == nil || reference.Id != "" {
		return reference, replyErr
	}
	var cancelled sessionReplyCancelled
	if errors.As(replyErr, &cancelled) || ctx.Err() != nil {
		return reference, replyErr
	}
	err, request := runtime.store.SessionEventGet(runtime.dbos, input.Event)
	if err != nil || request == nil || request.Kind != model.SessionEventKindAgentRequest {
		return reference, errors.Join(replyErr, err)
	}
	err, cancellation := runtime.replyCancellationRequested(runtime.dbos, input.Event)
	if err != nil || cancellation != nil {
		return reference, errors.Join(replyErr, err)
	}
	for _, terminal := range []string{model.SessionEventKindAgentSuccess, model.SessionEventKindAgentFailure} {
		err, existing := runtime.store.SessionEventChildGet(runtime.dbos, input.Event, terminal)
		if err != nil || existing != nil {
			return reference, errors.Join(replyErr, err)
		}
	}
	agentID, ok := request.Payload["agent"].(string)
	if !ok || agentID == "" {
		return reference, replyErr
	}
	payload, err := database.SessionEventPayloadFrom(model.AgentFailurePayload{Code: failureEventCode(replyErr)})
	if err != nil {
		return reference, errors.Join(replyErr, err)
	}
	failure := model.SessionEvent{
		Ref: model.SessionEventRef{Session: input.Event.Session}, Parent: &input.Event,
		Kind: model.SessionEventKindAgentFailure, AuthorAgent: &model.WorkspaceAgentRef{Workspace: input.Event.Session.Workspace, Id: agentID},
		Payload: payload,
	}
	if err, _ := runtime.persistAgentEvent(ctx, failure); err != nil {
		return reference, errors.Join(replyErr, fmt.Errorf("persist agent failure: %w", err))
	}
	return reference, replyErr
}

func failureEventCode(err error) string {
	var cancelled sessionReplyCancelled
	if errors.As(err, &cancelled) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, dbos.ErrWorkflowCancelled) || errors.Is(err, lisp.ErrInterrupted) {
		return "cancelled"
	}
	return "failed"
}

func (runtime *SessionEventReplyRuntime) replyAttempt(ctx dbos.Context, input SessionEventReplyInput) (model.SessionEventRef, error) {
	err, request := runtime.store.SessionEventGet(ctx, input.Event)
	if err != nil {
		return model.SessionEventRef{}, err
	}
	if request == nil {
		return model.SessionEventRef{}, fmt.Errorf("reply to session event %q: event not found", input.Event.Id)
	}
	if request.Kind != model.SessionEventKindAgentRequest || request.Parent == nil || request.AuthorPrincipal == nil {
		return model.SessionEventRef{}, fmt.Errorf("reply to session event %q: request is invalid", input.Event.Id)
	}
	err, message := runtime.store.SessionEventGet(ctx, *request.Parent)
	if err != nil {
		return model.SessionEventRef{}, err
	}
	if message == nil || message.Kind != model.SessionEventKindMessageText || message.AuthorPrincipal == nil || message.Ref.Session != request.Ref.Session {
		return model.SessionEventRef{}, fmt.Errorf("reply to session event %q: parent message is invalid", input.Event.Id)
	}
	preferred, _ := request.Payload["agent"].(string)
	if preferred == "" {
		return model.SessionEventRef{}, fmt.Errorf("reply to session event %q: target agent is unavailable", input.Event.Id)
	}
	err, selected := runtime.store.WorkspaceAgentModelGet(ctx, input.Event.Session.Workspace, preferred)
	if err != nil {
		return model.SessionEventRef{}, err
	}
	if err := runtime.replyCancellationCheck(ctx, input.Event); err != nil {
		if _, cancelled := err.(sessionReplyCancelled); cancelled {
			return model.SessionEventRef{}, nil
		}
		return model.SessionEventRef{}, err
	}
	if selected == nil {
		return model.SessionEventRef{}, fmt.Errorf("reply to session event %q: target agent is unavailable", input.Event.Id)
	}
	final := agentFinalReply{}
	switch selected.Protocol {
	case "builtin":
		err, thinking := runtime.thinkingStart(ctx, input.Event, selected.Ref, 0)
		if err != nil {
			return model.SessionEventRef{}, err
		}
		err, final.Text = BuiltinReply(selected.Model, selected.Parameters)
		if err != nil {
			return model.SessionEventRef{}, runtime.thinkingFinish(ctx, thinking, model.SessionEventKindThinkingFailure, err)
		}
		if err := runtime.thinkingFinish(ctx, thinking, model.SessionEventKindThinkingSuccess, nil); err != nil {
			return model.SessionEventRef{}, err
		}
	case "openai-chat-completions", "openai-responses":
		if message.AuthorPrincipal == nil {
			return model.SessionEventRef{}, fmt.Errorf("reply to session event %q: Lisp authorization requires a principal author", input.Event.Id)
		}
		err, final = runtime.openAIReply(ctx, input.Event, selected, message.AuthorPrincipal.Ref)
	default:
		err = fmt.Errorf("unsupported provider protocol %q", selected.Protocol)
	}
	if err != nil {
		if _, cancelled := err.(sessionReplyCancelled); cancelled {
			return model.SessionEventRef{}, nil
		}
		return model.SessionEventRef{}, err
	}
	if err := runtime.replyCancellationCheck(ctx, input.Event); err != nil {
		if _, cancelled := err.(sessionReplyCancelled); cancelled {
			return model.SessionEventRef{}, nil
		}
		return model.SessionEventRef{}, err
	}
	if len(final.Attachments) > 0 {
		err, references := runtime.store.SessionFileReferencesFilter(ctx, input.Event.Session, final.Attachments)
		if err == nil {
			final.Attachments = make([]string, len(references))
			for index, reference := range references {
				final.Attachments[index] = reference.ID
			}
		}
		if err != nil {
			return model.SessionEventRef{}, fmt.Errorf("filter agent reply attachments: %w", err)
		}
	}
	payload, err := database.SessionEventPayloadFrom(model.AgentSuccessPayload{Text: final.Text, Attachments: optionalStringSlice(final.Attachments)})
	if err != nil {
		return model.SessionEventRef{}, fmt.Errorf("encode agent success payload: %w", err)
	}
	event := model.SessionEvent{
		Ref:         model.SessionEventRef{Session: input.Event.Session},
		Parent:      &request.Ref,
		Kind:        model.SessionEventKindAgentSuccess,
		AuthorAgent: &selected.Ref,
		Payload:     payload,
	}
	err, stored := runtime.persistAgentEvent(ctx, event)
	if err != nil {
		return model.SessionEventRef{}, err
	}
	return stored, nil
}

func (runtime *SessionEventReplyRuntime) openAIReply(ctx dbos.Context, parent model.SessionEventRef, selected *database.WorkspaceAgentModel, principal model.PrincipalRef) (error, agentFinalReply) {
	agent := selected.Ref
	callCount := 0
	for round := 0; ; {
		if err := runtime.replyCancellationCheck(ctx, parent); err != nil {
			return err, agentFinalReply{}
		}
		err, selected := runtime.currentWorkspaceAgentModel(ctx, agent)
		if err != nil {
			return err, agentFinalReply{}
		}
		err, thinking := runtime.thinkingStart(ctx, parent, selected.Ref, round)
		if err != nil {
			return err, agentFinalReply{}
		}
		turn := agentProviderTurn{}
		for {
			err, selected = runtime.currentWorkspaceAgentModel(ctx, agent)
			if err != nil {
				return runtime.thinkingFinish(ctx, thinking, model.SessionEventKindThinkingFailure, err), agentFinalReply{}
			}
			err, turn = runtime.agentProviderTurn(ctx, parent, thinking, selected, principal)
			if err == nil {
				break
			}
			var retryable *openAIRetryableError
			if !errors.As(err, &retryable) {
				return runtime.thinkingFinishWithMetrics(ctx, thinking, model.SessionEventKindThinkingFailure, err, turn.Metrics), agentFinalReply{}
			}
			if _, err := dbos.Sleep(ctx, retryable.RetryAfter()); err != nil {
				return runtime.thinkingFinish(ctx, thinking, model.SessionEventKindThinkingFailure, err), agentFinalReply{}
			}
		}
		if turn.Final != nil {
			if err := runtime.thinkingFinishWithMetrics(ctx, thinking, model.SessionEventKindThinkingSuccess, nil, turn.Metrics); err != nil {
				return err, agentFinalReply{}
			}
			return nil, *turn.Final
		}
		if round >= selected.MaxTurns {
			err := fmt.Errorf("agent provider completion exceeded turn limit")
			return runtime.thinkingFinishWithMetrics(ctx, thinking, model.SessionEventKindThinkingFailure, err, turn.Metrics), agentFinalReply{}
		}
		if err := runtime.thinkingFinishWithMetrics(ctx, thinking, model.SessionEventKindThinkingSuccess, nil, turn.Metrics); err != nil {
			return err, agentFinalReply{}
		}
		if err := runtime.replyCancellationCheck(ctx, parent); err != nil {
			return err, agentFinalReply{}
		}
		profile, err := selectedMCMTRProfile(selected)
		if err != nil {
			return err, agentFinalReply{}
		}
		if !mcmtrNativeCallsFit(turn.Calls, profile.BufferBytes) {
			err := fmt.Errorf("OpenAI-compatible completion requested a tool batch that exceeds the MCMTR tool high-tier buffer")
			return runtime.thinkingFinish(ctx, thinking, model.SessionEventKindThinkingFailure, err), agentFinalReply{}
		}
		_, err = runtime.runLispCalls(ctx, parent, selected, principal, round, callCount, turn.Calls)
		if err != nil {
			return err, agentFinalReply{}
		}
		callCount += len(turn.Calls)
		round++
	}
}

func (runtime *SessionEventReplyRuntime) currentWorkspaceAgentModel(ctx context.Context, agent model.WorkspaceAgentRef) (error, *database.WorkspaceAgentModel) {
	err, selected := runtime.store.WorkspaceAgentModelGet(ctx, agent.Workspace, agent.Id)
	if err != nil {
		return err, nil
	}
	if selected == nil {
		return fmt.Errorf("workspace agent %q is unavailable", agent.Id), nil
	}
	return nil, selected
}

func (runtime *SessionEventReplyRuntime) agentProviderTurn(ctx dbos.Context, parent model.SessionEventRef, thinking model.SessionEvent, selected *database.WorkspaceAgentModel, principal model.PrincipalRef) (error, agentProviderTurn) {
	if selected.Protocol == "builtin" {
		err, text := BuiltinReply(selected.Model, selected.Parameters)
		if err != nil {
			return err, agentProviderTurn{}
		}
		return nil, agentProviderTurn{Final: &agentFinalReply{Text: text}}
	}
	err, project := runtime.store.SessionProjectGet(ctx, parent.Session)
	if err != nil {
		return err, agentProviderTurn{}
	}
	projectChat := project != nil
	switch selected.Protocol {
	case "openai-chat-completions":
		if selected.BaseURL == nil || selected.Keychain == nil || selected.APIKey == nil {
			return fmt.Errorf("reply with OpenAI-compatible provider %q: missing credentials", selected.ProviderID), agentProviderTurn{}
		}
		if err := runtime.mcmtrContextStart(ctx, parent, selected, projectChat); err != nil {
			return err, agentProviderTurn{}
		}
		err, messages := runtime.agentContext(ctx, parent, selected, projectChat)
		if err != nil {
			return err, agentProviderTurn{}
		}
		err, reasoningEffort := openAICompatibleReasoningEffort(selected.Parameters)
		if err != nil {
			return err, agentProviderTurn{}
		}
		request := openAICompatibleRequest{
			Model: selected.Model, Messages: openAIRequestMessages(selected, messages, projectChat), Tools: []openAICompatibleTool{openAICompatibleLispTool()}, ResponseFormat: openAIFinalReplyResponseFormat(), ParallelToolCalls: true, ReasoningEffort: reasoningEffort, MaxTokens: selected.MaxOutputTokens,
		}
		err, claim := runtime.agentRateLimitClaim(ctx, parent, thinking, selected, principal, request)
		if err != nil {
			return err, agentProviderTurn{}
		}
		completion, err := runtime.openAICompatibleComplete(ctx, selected, request)
		inputTokens, outputTokens := agentUsageTokens(err, completion.Metrics)
		if settleErr := runtime.agentRateLimitSettle(ctx, claim, inputTokens, outputTokens); settleErr != nil {
			return settleErr, agentProviderTurn{}
		}
		if err != nil {
			return err, agentProviderTurn{}
		}
		if len(completion.Message.ToolCalls) > 0 {
			return nil, agentProviderTurn{Calls: completion.Message.ToolCalls, Metrics: completion.Metrics}
		}
		final, err := openAIFinalReply(completion.Message.Content)
		if err != nil {
			logInvalidOpenAIFinalReply(selected.ProviderID, parent, completion.Message.Content)
			return err, agentProviderTurn{Metrics: completion.Metrics}
		}
		return nil, agentProviderTurn{Final: &final, Metrics: completion.Metrics}
	case "openai-responses":
		if selected.BaseURL == nil || selected.Keychain == nil || selected.APIKey == nil {
			return fmt.Errorf("reply with OpenAI-compatible provider %q: missing credentials", selected.ProviderID), agentProviderTurn{}
		}
		if err := runtime.mcmtrContextStart(ctx, parent, selected, projectChat); err != nil {
			return err, agentProviderTurn{}
		}
		err, messages := runtime.agentContext(ctx, parent, selected, projectChat)
		if err != nil {
			return err, agentProviderTurn{}
		}
		err, reasoningEffort := openAICompatibleReasoningEffort(selected.Parameters)
		if err != nil {
			return err, agentProviderTurn{}
		}
		var reasoning *openAIResponsesReasoning
		if reasoningEffort != "" {
			reasoning = &openAIResponsesReasoning{Effort: reasoningEffort}
		}
		input := openAIResponsesInput(messages)
		request := openAIResponsesRequest{
			Model: selected.Model, Instructions: openAISystemPromptFor(selected, projectChat), Input: input, Tools: []openAIResponsesTool{openAIResponsesLispTool()}, Text: openAIResponsesFinalReplyText(), ParallelToolCalls: true, Reasoning: reasoning, MaxOutputTokens: selected.MaxOutputTokens,
		}
		err, claim := runtime.agentRateLimitClaim(ctx, parent, thinking, selected, principal, request)
		if err != nil {
			return err, agentProviderTurn{}
		}
		response, err := runtime.openAIResponsesComplete(ctx, selected, request)
		inputTokens, outputTokens := agentUsageTokens(err, response.Metrics)
		if settleErr := runtime.agentRateLimitSettle(ctx, claim, inputTokens, outputTokens); settleErr != nil {
			return settleErr, agentProviderTurn{}
		}
		if err != nil {
			return err, agentProviderTurn{}
		}
		outputs := make([]openAIResponsesOutput, len(response.Output))
		calls := make([]openAICompatibleToolCall, 0, 1)
		for index, raw := range response.Output {
			var output openAIResponsesOutput
			if err := json.Unmarshal(raw, &output); err != nil {
				err := fmt.Errorf("decode OpenAI Responses output: %w", err)
				logInvalidOpenAIResponsesFinalReply(selected.ProviderID, parent, input, response.Output)
				return err, agentProviderTurn{Metrics: response.Metrics}
			}
			outputs[index] = output
			if output.Type == "function_call" {
				call := openAICompatibleToolCall{ID: output.CallID, Type: "function"}
				call.Function.Name = output.Name
				call.Function.Arguments = output.Arguments
				calls = append(calls, call)
			}
		}
		if len(calls) > 0 {
			return nil, agentProviderTurn{Calls: calls, Metrics: response.Metrics}
		}
		final, err := openAIResponsesFinalReply(outputs)
		if err != nil {
			logInvalidOpenAIResponsesFinalReply(selected.ProviderID, parent, input, response.Output)
			return err, agentProviderTurn{Metrics: response.Metrics}
		}
		return nil, agentProviderTurn{Final: &final, Metrics: response.Metrics}
	default:
		return fmt.Errorf("unsupported provider protocol %q", selected.Protocol), agentProviderTurn{}
	}
}

func (runtime *SessionEventReplyRuntime) openAICompatibleComplete(ctx dbos.Context, selected *database.WorkspaceAgentModel, request openAICompatibleRequest) (openAICompatibleCompletion, error) {
	err, apiKey := runtime.openAIAPIKey(ctx, selected)
	if err != nil {
		return openAICompatibleCompletion{}, err
	}
	replyErr, reply := OpenAICompatibleComplete(ctx, &http.Client{Timeout: time.Minute}, *selected.BaseURL, string(apiKey), request)
	clear(apiKey)
	return reply, replyErr
}

func (runtime *SessionEventReplyRuntime) openAIResponsesComplete(ctx dbos.Context, selected *database.WorkspaceAgentModel, request openAIResponsesRequest) (openAIResponsesResponse, error) {
	err, apiKey := runtime.openAIAPIKey(ctx, selected)
	if err != nil {
		return openAIResponsesResponse{}, err
	}
	replyErr, reply := OpenAIResponsesComplete(ctx, &http.Client{Timeout: time.Minute}, *selected.BaseURL, string(apiKey), request)
	clear(apiKey)
	return reply, replyErr
}

func (runtime *SessionEventReplyRuntime) openAIAPIKey(ctx context.Context, selected *database.WorkspaceAgentModel) (error, []byte) {
	err, encrypted := keychain.ParseKey(*selected.APIKey)
	if err != nil {
		return fmt.Errorf("parse API key for provider %q: %w", selected.ProviderID, err), nil
	}
	err, keys := runtime.keyring.Get(ctx, []model.KeychainRef{*selected.Keychain})
	if err != nil {
		return fmt.Errorf("get keychain for provider %q: %w", selected.ProviderID, err), nil
	}
	key := keys[*selected.Keychain]
	associatedDataErr, associatedData := agentProviderAssociatedData(selected.ProviderID, selected.ProviderAlias, encrypted.AAD)
	if associatedDataErr != nil {
		clear(key)
		clear(keys)
		return fmt.Errorf("select API key AAD for provider %q: %w", selected.ProviderID, associatedDataErr), nil
	}
	decryptedErr, apiKey := keychain.Open(key, associatedData, encrypted)
	clear(key)
	clear(keys)
	if decryptedErr != nil {
		return fmt.Errorf("decrypt API key for provider %q: %w", selected.ProviderID, decryptedErr), nil
	}
	return nil, apiKey
}

func agentProviderAssociatedData(id string, alias *string, selector string) (error, []byte) {
	switch selector {
	case keychain.AADID:
		return nil, []byte("gh=v1|agent-provider|id=" + id)
	case keychain.AADAlias:
		if alias == nil {
			return fmt.Errorf("provider alias is unavailable"), nil
		}
		return nil, []byte("gh=v1|agent-provider|alias=" + *alias)
	default:
		return fmt.Errorf("unsupported AAD selector %q", selector), nil
	}
}

func (runtime *SessionEventReplyRuntime) toolCall(ctx dbos.Context, input SessionToolCallInput) (model.SessionEventRef, error) {
	reference, callErr := runtime.toolCallAttempt(ctx, input)
	if callErr == nil || reference.Id != "" || ctx.Err() != nil {
		return reference, callErr
	}
	err, request := runtime.store.SessionEventGet(runtime.dbos, input.Request)
	if err != nil || request == nil || request.Kind != model.SessionEventKindToolRequest || request.AuthorAgent == nil {
		return reference, errors.Join(callErr, err)
	}
	if request.Parent != nil {
		err, cancellation := runtime.replyCancellationRequested(runtime.dbos, *request.Parent)
		if err != nil || cancellation != nil {
			return reference, errors.Join(callErr, err)
		}
	}
	for _, terminal := range []string{model.SessionEventKindToolSuccess, model.SessionEventKindToolFailure} {
		err, existing := runtime.store.SessionEventChildGet(runtime.dbos, input.Request, terminal)
		if err != nil || existing != nil {
			return reference, errors.Join(callErr, err)
		}
	}
	callID, _ := request.Payload["call_id"].(string)
	if callID == "" {
		callID = "unknown"
	}
	payload, err := database.SessionEventPayloadFrom(model.ToolFailurePayload{
		Name: "lisp", CallId: callID, Code: failureEventCode(callErr), Output: callErr.Error(),
	})
	if err != nil {
		return reference, errors.Join(callErr, err)
	}
	failure := model.SessionEvent{
		Ref: model.SessionEventRef{Session: request.Ref.Session}, Parent: &request.Ref,
		Kind: model.SessionEventKindToolFailure, AuthorAgent: request.AuthorAgent, Payload: payload,
		CreatedAt: toolOutputCreatedAt(request.CreatedAt),
	}
	if err, reference = runtime.persistAgentEvent(ctx, failure); err != nil {
		return reference, errors.Join(callErr, fmt.Errorf("persist tool failure: %w", err))
	}
	return reference, callErr
}

func (runtime *SessionEventReplyRuntime) toolCallAttempt(ctx dbos.Context, input SessionToolCallInput) (model.SessionEventRef, error) {
	err, request := runtime.store.SessionEventGet(ctx, input.Request)
	if err != nil {
		return model.SessionEventRef{}, err
	}
	if request == nil || request.Kind != model.SessionEventKindToolRequest || request.Parent == nil || request.AuthorAgent == nil {
		return model.SessionEventRef{}, fmt.Errorf("session tool call request is invalid")
	}
	err, parent := runtime.store.SessionEventGet(ctx, *request.Parent)
	if err != nil {
		return model.SessionEventRef{}, err
	}
	if parent == nil || parent.AuthorPrincipal == nil {
		return model.SessionEventRef{}, fmt.Errorf("session tool call principal is unavailable")
	}
	callID, callIDOK := request.Payload["call_id"].(string)
	code, codeOK := request.Payload["code"].(string)
	name, nameOK := request.Payload["name"].(string)
	if !callIDOK || !codeOK || !nameOK || name != "lisp" {
		return model.SessionEventRef{}, fmt.Errorf("session tool call payload is invalid")
	}
	err, selected := runtime.store.WorkspaceAgentModelGet(ctx, request.Ref.Session.Workspace, request.AuthorAgent.Id)
	if err != nil {
		return model.SessionEventRef{}, err
	}
	if selected == nil || selected.Ref != *request.AuthorAgent {
		return model.SessionEventRef{}, fmt.Errorf("session tool call agent is unavailable")
	}
	agent := *request.AuthorAgent
	principal := parent.AuthorPrincipal.Ref
	err, project := runtime.store.SessionProjectGet(ctx, request.Ref.Session)
	if err != nil {
		return model.SessionEventRef{}, err
	}
	call, err := diagnostics.Begin("agent.tool_call.evaluate", "")
	if err != nil {
		return runtime.toolCallFinish(ctx, *request, agent, callID, sessionToolCallExecution{Kind: model.SessionEventKindToolFailure, Code: "execution_failed", Output: err.Error()})
	}
	modules := []lisp.HostModule{}
	if project != nil {
		modules = append(modules,
			runtime.projectInfoModule(ctx, request.Ref.Session, principal),
			runtime.projectFilesModule(ctx, request.Ref.Session, principal),
			runtime.projectNotesModule(ctx, request.Ref.Session, principal, agent),
			runtime.projectTasksModule(ctx, request.Ref.Session, principal, agent),
			runtime.projectSecretsModule(ctx, request.Ref.Session, principal),
			runtime.projectRecordsModule(ctx, request.Ref.Session, principal, agent),
		)
	}
	modules = append(modules,
		runtime.sessionFilesModule(ctx, request.Ref.Session, principal),
		runtime.sessionNotesModule(ctx, request.Ref.Session, principal, agent),
		runtime.sessionTasksModule(ctx, request.Ref.Session, principal, agent),
		runtime.sessionSecretsModule(ctx, request.Ref.Session, principal),
		runtime.sessionEventsModule(ctx, *request.Parent, agent, principal),
		NewInputModule(func(form inputform.Form) (error, string) { return runtime.awaitInput(ctx, *request, agent, form) }),
		NewPolicyModule(func(description string) error { return runtime.awaitApproval(ctx, *request, agent, description) }),
		NewWebModule(),
	)
	evaluationContext, cancelEvaluation := context.WithCancel(ctx)
	stopCancellationWatch := runtime.watchToolCallCancellation(request.Parent, cancelEvaluation)
	defer stopCancellationWatch()
	defer cancelEvaluation()
	evalErr, result := lisp.Evaluate(code, lisp.EvalOptions{Context: evaluationContext, Prelude: agentPreludeFor(selected, project != nil), HostModules: modules})
	evalErr = call.End(evalErr)
	execution := sessionToolCallExecution{}
	if errors.Is(evalErr, lisp.ErrInterrupted) {
		execution = sessionToolCallExecution{Kind: model.SessionEventKindToolFailure, Code: "cancelled", Output: "execution cancelled"}
	} else if evalErr != nil {
		execution = sessionToolCallExecution{Kind: model.SessionEventKindToolFailure, Code: "execution_failed", Output: evalErr.Error()}
	} else {
		execution = sessionToolCallExecution{Kind: model.SessionEventKindToolSuccess, Output: result.String()}
	}
	return runtime.toolCallFinish(ctx, *request, agent, callID, execution)
}

func (runtime *SessionEventReplyRuntime) watchToolCallCancellation(parent *model.SessionEventRef, cancel context.CancelFunc) func() {
	if parent == nil {
		return func() {}
	}
	return watchCancellation(func(ctx context.Context) bool {
		err, request := runtime.replyCancellationRequested(ctx, *parent)
		return err == nil && request != nil
	}, cancel)
}

func watchCancellation(requested func(context.Context) bool, cancel context.CancelFunc) func() {
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(lispCancellationPoll)
		defer ticker.Stop()
		for {
			if requested(ctx) {
				cancel()
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() {
		stop()
		<-done
	}
}

func sessionEventReadRange(event model.SessionEvent, offset, length int64) (error, []byte) {
	output, available := sessionsearch.EventReadableBody(event)
	if !available {
		return fmt.Errorf("read session event: unavailable"), nil
	}
	contents := []byte(output)
	if offset > int64(len(contents)) {
		return fmt.Errorf("read session event: unavailable"), nil
	}
	end := offset + length
	if end > int64(len(contents)) {
		end = int64(len(contents))
	}
	return nil, append([]byte(nil), contents[offset:end]...)
}

func (runtime *SessionEventReplyRuntime) toolCallFinish(ctx dbos.Context, request model.SessionEvent, agent model.WorkspaceAgentRef, callID string, execution sessionToolCallExecution) (model.SessionEventRef, error) {
	var payload map[string]interface{}
	var err error
	if execution.Kind == model.SessionEventKindToolSuccess {
		payload, err = database.SessionEventPayloadFrom(model.ToolSuccessPayload{Name: "lisp", CallId: callID, Output: execution.Output})
	} else {
		payload, err = database.SessionEventPayloadFrom(model.ToolFailurePayload{Name: "lisp", CallId: callID, Code: execution.Code, Output: execution.Output})
	}
	if err != nil {
		return model.SessionEventRef{}, fmt.Errorf("encode tool terminal payload: %w", err)
	}
	resultEvent := model.SessionEvent{
		Ref: model.SessionEventRef{Session: request.Ref.Session}, Parent: &request.Ref, Kind: execution.Kind, AuthorAgent: &agent,
		Payload: payload, CreatedAt: toolOutputCreatedAt(request.CreatedAt),
	}
	err, reference := runtime.persistAgentEvent(ctx, resultEvent)
	if err != nil {
		return model.SessionEventRef{}, err
	}
	return reference, nil
}

func optionalStringSlice(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	return &values
}

func (runtime *SessionEventReplyRuntime) sessionNoteCreate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) SessionNoteCreate {
	return func(title, description, body string, sensitive bool) (error, SessionNote) {
		note, err := dbos.RunAsStep(ctx, func(step context.Context) (SessionNote, error) {
			id, err := typed_id.New(typed_id.SessionNote)
			if err != nil {
				return SessionNote{}, fmt.Errorf("generate session note ID: %w", err)
			}
			err, note := runtime.store.SessionNoteCreate(step, model.SessionNote{
				Ref: model.SessionNoteRef{Session: session, Id: id}, AuthorAgent: &agent, Title: title, Description: description, Body: body, Sensitive: sensitive,
			}, principal)
			if err != nil {
				return SessionNote{}, err
			}
			return SessionNote{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Sensitive: note.Sensitive, AuthorID: note.AuthorAgent.Id, CreatedAt: note.CreatedAt, Revision: note.Revision}, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-session-note-create"))
		return err, note
	}
}

func (runtime *SessionEventReplyRuntime) sessionFileCreate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) SessionFileCreate {
	return func(name, mediaType string, source io.Reader) (error, string) {
		id, err := dbos.RunAsStep(ctx, func(step context.Context) (string, error) {
			fileID, err := typed_id.New(typed_id.SessionFile)
			if err != nil {
				return "", fmt.Errorf("generate session file ID: %w", err)
			}
			storageObjectID, err := typed_id.New(typed_id.StorageObject)
			if err != nil {
				return "", fmt.Errorf("generate storage object ID: %w", err)
			}
			mediaTypeValue := mediaType
			err, file, objectID := runtime.store.SessionFileCreate(step, model.SessionFile{
				Ref: model.SessionFileRef{Session: session, Id: fileID}, Name: name, MediaType: &mediaTypeValue, Enabled: true,
			}, storageObjectID, principal)
			if err != nil {
				return "", err
			}
			if err := runtime.storage.Put(step, objectID, source, -1); err != nil {
				return "", err
			}
			if err := runtime.storage.Finish(step, objectID); err != nil {
				return "", err
			}
			return file.Ref.Id, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-session-file-create"))
		return err, id
	}
}

func (runtime *SessionEventReplyRuntime) projectFileCreate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) ProjectFileCreate {
	return func(name, mediaType string, source io.Reader) (error, string) {
		id, err := dbos.RunAsStep(ctx, func(step context.Context) (string, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectFileCreate)
			if err != nil {
				return "", err
			}
			fileID, err := typed_id.New(typed_id.ProjectFile)
			if err != nil {
				return "", fmt.Errorf("generate project file ID: %w", err)
			}
			storageObjectID, err := typed_id.New(typed_id.StorageObject)
			if err != nil {
				return "", fmt.Errorf("generate storage object ID: %w", err)
			}
			mediaTypeValue := mediaType
			err, file, objectID := runtime.store.ProjectFileCreate(step, model.ProjectFile{
				Ref: model.ProjectFileRef{Project: *project, Id: fileID}, Name: name, MediaType: &mediaTypeValue, Enabled: true,
			}, storageObjectID, principal)
			if err != nil {
				return "", err
			}
			if err := runtime.storage.Put(step, objectID, source, -1); err != nil {
				return "", err
			}
			if err := runtime.storage.Finish(step, objectID); err != nil {
				return "", err
			}
			err, finished, _ := runtime.store.ProjectFileFinish(step, file.Ref, principal)
			if err != nil {
				return "", err
			}
			if finished == nil {
				return "", fmt.Errorf("finish project file: unavailable")
			}
			return finished.Ref.Id, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-project-file-create"))
		return err, id
	}
}

func (runtime *SessionEventReplyRuntime) sessionFileUpdate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) FileUpdate {
	return func(id, name string) (error, File) {
		file, err := dbos.RunAsStep(ctx, func(step context.Context) (File, error) {
			err, roles := runtime.store.SessionRolesGet(step, session, principal)
			if err != nil {
				return File{}, err
			}
			if !authz.SessionAllows(roles, authz.SessionFileUpdate) {
				return File{}, fmt.Errorf("update session file: permission denied")
			}
			err, updated, object := runtime.store.SessionFileUpdate(step, model.SessionFileRef{Session: session, Id: id}, database.FileUpdate{Name: name}, principal)
			if err != nil || updated == nil || object == nil {
				if err == nil {
					err = fmt.Errorf("update session file: unavailable")
				}
				return File{}, err
			}
			return File{ID: updated.Ref.Id, Name: updated.Name, MediaType: updated.MediaType, Size: object.Size, Fingerprint: fmt.Sprintf("sha256:%x", object.SHA256)}, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-session-file-update"))
		return err, file
	}
}

func (runtime *SessionEventReplyRuntime) sessionFileRemove(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) FileRemove {
	return func(id string) (error, bool) {
		removed, err := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
			err, roles := runtime.store.SessionRolesGet(step, session, principal)
			if err != nil {
				return false, err
			}
			if !authz.SessionAllows(roles, authz.SessionFileRemove) {
				return false, fmt.Errorf("remove session file: permission denied")
			}
			err, removed := runtime.store.SessionFileRemove(step, model.SessionFileRef{Session: session, Id: id}, principal)
			return removed, err
		}, dbos.WithStepName("gatehouse.session-tool-call-session-file-remove"))
		return err, removed
	}
}

func (runtime *SessionEventReplyRuntime) projectFileUpdate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) FileUpdate {
	return func(id, name string) (error, File) {
		file, err := dbos.RunAsStep(ctx, func(step context.Context) (File, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectFileUpdate)
			if err != nil {
				return File{}, fmt.Errorf("update project file: %w", err)
			}
			err, updated, object := runtime.store.ProjectFileUpdate(step, model.ProjectFileRef{Project: *project, Id: id}, database.FileUpdate{Name: name}, principal)
			if err != nil || updated == nil || object == nil {
				if err == nil {
					err = fmt.Errorf("update project file: unavailable")
				}
				return File{}, err
			}
			return File{ID: updated.Ref.Id, Name: updated.Name, MediaType: updated.MediaType, Size: object.Size, Fingerprint: fmt.Sprintf("sha256:%x", object.SHA256)}, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-project-file-update"))
		return err, file
	}
}

func (runtime *SessionEventReplyRuntime) projectFileRemove(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) FileRemove {
	return func(id string) (error, bool) {
		removed, err := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectFileRemove)
			if err != nil {
				return false, fmt.Errorf("remove project file: %w", err)
			}
			err, removed := runtime.store.ProjectFileRemove(step, model.ProjectFileRef{Project: *project, Id: id}, principal)
			return removed, err
		}, dbos.WithStepName("gatehouse.session-tool-call-project-file-remove"))
		return err, removed
	}
}

func (runtime *SessionEventReplyRuntime) sessionNoteRemove(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) NoteRemove {
	return func(id string) (error, bool) {
		removed, err := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
			err, removed := runtime.store.SessionNoteRemove(step, model.SessionNoteRef{Session: session, Id: id}, principal)
			return removed, err
		}, dbos.WithStepName("gatehouse.session-tool-call-session-note-remove"))
		return err, removed
	}
}

func (runtime *SessionEventReplyRuntime) sessionNoteUpdate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) SessionNoteUpdate {
	return func(id, title, description, body string, sensitive bool) (error, SessionNote) {
		note, err := dbos.RunAsStep(ctx, func(step context.Context) (SessionNote, error) {
			err, updated := runtime.store.SessionNoteDetailsSetAs(step, model.SessionNoteRef{Session: session, Id: id}, principal, database.NoteAuthor{Agent: &agent}, sensitive, &title, &description, &body)
			if err != nil || updated == nil {
				if err == nil {
					err = fmt.Errorf("update session note: unavailable")
				}
				return SessionNote{}, err
			}
			return SessionNote{ID: updated.Note.Ref.Id, Title: updated.Note.Title, Description: updated.Note.Description, Sensitive: updated.Note.Sensitive, AuthorID: noteAuthorID(updated.Note.AuthorPrincipal, updated.Note.AuthorAgent, updated.Note.AuthorGateway), CreatedAt: updated.Note.CreatedAt, Revision: updated.Note.Revision}, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-session-note-update"))
		return err, note
	}
}

func (runtime *SessionEventReplyRuntime) sessionTaskCreate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) TaskCreate {
	return func(title, description, status string, sensitive bool) (error, Task) {
		task, err := dbos.RunAsStep(ctx, func(step context.Context) (Task, error) {
			id, err := typed_id.New(typed_id.SessionTask)
			if err != nil {
				return Task{}, fmt.Errorf("generate session task ID: %w", err)
			}
			err, created := runtime.store.SessionTaskCreate(step, model.SessionTask{
				Ref: model.SessionTaskRef{Session: session, Id: id}, Title: title, Description: &description, Sensitive: sensitive, Status: status,
				CreatorAgent: &agent, UpdaterAgent: &agent,
			}, principal)
			if err != nil {
				return Task{}, err
			}
			return sessionTaskValue(created), nil
		}, dbos.WithStepName("gatehouse.session-tool-call-session-task-create"))
		return err, task
	}
}

func (runtime *SessionEventReplyRuntime) sessionTaskRead(ctx context.Context, session model.SessionRef, principal model.PrincipalRef) TaskRead {
	return func(id string, offset, length int64) (error, []byte, bool) {
		err, detail := runtime.store.SessionTaskGet(ctx, model.SessionTaskRef{Session: session, Id: id}, principal)
		if err != nil || detail == nil {
			if err != nil {
				return err, nil, false
			}
			return fmt.Errorf("read session task: unavailable"), nil, false
		}
		description := ""
		if detail.Description != nil {
			description = *detail.Description
		}
		contents := []byte(description)
		if offset > int64(len(contents)) {
			return fmt.Errorf("read session task: offset is unavailable"), nil, false
		}
		end := offset + length
		if end > int64(len(contents)) {
			end = int64(len(contents))
		}
		return nil, append([]byte(nil), contents[offset:end]...), detail.Sensitive
	}
}

func (runtime *SessionEventReplyRuntime) sessionTaskUpdate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) TaskUpdate {
	return func(id, title, description, status string, sensitive bool) (error, Task) {
		task, err := dbos.RunAsStep(ctx, func(step context.Context) (Task, error) {
			err, updated := runtime.store.SessionTaskDetailsSetAs(step, model.SessionTaskRef{Session: session, Id: id}, principal, database.TaskAuthor{Agent: &agent}, sensitive, status, title, &description)
			if err != nil || updated == nil {
				if err == nil {
					err = fmt.Errorf("update session task: unavailable")
				}
				return Task{}, err
			}
			return sessionTaskValue(*updated), nil
		}, dbos.WithStepName("gatehouse.session-tool-call-session-task-update"))
		return err, task
	}
}

func (runtime *SessionEventReplyRuntime) sessionTaskRemove(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) TaskRemove {
	return func(id string) (error, bool) {
		removed, err := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
			err, removed := runtime.store.SessionTaskRemove(step, model.SessionTaskRef{Session: session, Id: id}, principal)
			return removed, err
		}, dbos.WithStepName("gatehouse.session-tool-call-session-task-remove"))
		return err, removed
	}
}

func (runtime *SessionEventReplyRuntime) sessionProjectGet(ctx context.Context, session model.SessionRef, principal model.PrincipalRef) (*model.ProjectRef, error) {
	err, project := runtime.store.SessionProjectGet(ctx, session)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, fmt.Errorf("project is unavailable")
	}
	err, available := runtime.store.ProjectGet(ctx, *project, principal)
	if err != nil {
		return nil, err
	}
	if available == nil {
		return nil, fmt.Errorf("project is unavailable")
	}
	return project, nil
}

func (runtime *SessionEventReplyRuntime) sessionProjectActionGet(ctx context.Context, session model.SessionRef, principal model.PrincipalRef, action authz.ProjectAction) (*model.ProjectRef, error) {
	project, err := runtime.sessionProjectGet(ctx, session, principal)
	if err != nil {
		return nil, err
	}
	err, roles := runtime.store.ProjectRolesGet(ctx, *project, principal)
	if err != nil {
		return nil, err
	}
	if !authz.ProjectAllows(roles, action) {
		return nil, fmt.Errorf("project action is unavailable")
	}
	return project, nil
}

func (runtime *SessionEventReplyRuntime) projectNoteCreate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) ProjectNoteCreate {
	return func(title, description, body string, sensitive bool) (error, ProjectNote) {
		note, err := dbos.RunAsStep(ctx, func(step context.Context) (ProjectNote, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectNoteCreate)
			if err != nil {
				return ProjectNote{}, fmt.Errorf("create project note: %w", err)
			}
			id, err := typed_id.New(typed_id.ProjectNote)
			if err != nil {
				return ProjectNote{}, fmt.Errorf("generate project note ID: %w", err)
			}
			err, note := runtime.store.ProjectNoteCreate(step, model.ProjectNote{
				Ref: model.ProjectNoteRef{Project: *project, Id: id}, AuthorAgent: &agent, Title: title, Description: description, Body: body, Sensitive: sensitive,
			}, principal)
			if err != nil {
				return ProjectNote{}, err
			}
			return ProjectNote{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Sensitive: note.Sensitive, AuthorID: note.AuthorAgent.Id, CreatedAt: note.CreatedAt, Revision: note.Revision}, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-project-note-create"))
		return err, note
	}
}

func (runtime *SessionEventReplyRuntime) projectTaskCreate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) TaskCreate {
	return func(title, description, status string, sensitive bool) (error, Task) {
		task, err := dbos.RunAsStep(ctx, func(step context.Context) (Task, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectTaskCreate)
			if err != nil {
				err = fmt.Errorf("create project task: %w", err)
				return Task{}, err
			}
			id, err := typed_id.New(typed_id.ProjectTask)
			if err != nil {
				return Task{}, fmt.Errorf("generate project task ID: %w", err)
			}
			err, created := runtime.store.ProjectTaskCreate(step, model.ProjectTask{
				Ref: model.ProjectTaskRef{Project: *project, Id: id}, Title: title, Description: &description, Sensitive: sensitive, Status: status,
				CreatorAgent: &agent, UpdaterAgent: &agent,
			}, principal)
			if err != nil {
				return Task{}, err
			}
			return projectTaskValue(created), nil
		}, dbos.WithStepName("gatehouse.session-tool-call-project-task-create"))
		return err, task
	}
}

func (runtime *SessionEventReplyRuntime) projectTaskRead(ctx context.Context, session model.SessionRef, principal model.PrincipalRef) TaskRead {
	return func(id string, offset, length int64) (error, []byte, bool) {
		project, err := runtime.sessionProjectGet(ctx, session, principal)
		if err != nil {
			return fmt.Errorf("read project task: %w", err), nil, false
		}
		err, detail := runtime.store.ProjectTaskGet(ctx, model.ProjectTaskRef{Project: *project, Id: id}, principal)
		if err != nil || detail == nil {
			if err != nil {
				return err, nil, false
			}
			return fmt.Errorf("read project task: unavailable"), nil, false
		}
		description := ""
		if detail.Description != nil {
			description = *detail.Description
		}
		contents := []byte(description)
		if offset > int64(len(contents)) {
			return fmt.Errorf("read project task: offset is unavailable"), nil, false
		}
		end := offset + length
		if end > int64(len(contents)) {
			end = int64(len(contents))
		}
		return nil, append([]byte(nil), contents[offset:end]...), detail.Sensitive
	}
}

func (runtime *SessionEventReplyRuntime) projectTaskUpdate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) TaskUpdate {
	return func(id, title, description, status string, sensitive bool) (error, Task) {
		task, err := dbos.RunAsStep(ctx, func(step context.Context) (Task, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectTaskEdit)
			if err != nil {
				err = fmt.Errorf("update project task: %w", err)
				return Task{}, err
			}
			err, updated := runtime.store.ProjectTaskDetailsSetAs(step, model.ProjectTaskRef{Project: *project, Id: id}, principal, database.TaskAuthor{Agent: &agent}, sensitive, status, title, &description)
			if err != nil || updated == nil {
				if err == nil {
					err = fmt.Errorf("update project task: unavailable")
				}
				return Task{}, err
			}
			return projectTaskValue(*updated), nil
		}, dbos.WithStepName("gatehouse.session-tool-call-project-task-update"))
		return err, task
	}
}

func (runtime *SessionEventReplyRuntime) projectTaskRemove(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) TaskRemove {
	return func(id string) (error, bool) {
		removed, err := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectTaskRemove)
			if err != nil {
				err = fmt.Errorf("remove project task: %w", err)
				return false, err
			}
			err, removed := runtime.store.ProjectTaskRemove(step, model.ProjectTaskRef{Project: *project, Id: id}, principal)
			return removed, err
		}, dbos.WithStepName("gatehouse.session-tool-call-project-task-remove"))
		return err, removed
	}
}

func (runtime *SessionEventReplyRuntime) projectNoteRemove(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) NoteRemove {
	return func(id string) (error, bool) {
		removed, err := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectNoteRemove)
			if err != nil {
				return false, fmt.Errorf("remove project note: %w", err)
			}
			err, removed := runtime.store.ProjectNoteRemove(step, model.ProjectNoteRef{Project: *project, Id: id}, principal)
			return removed, err
		}, dbos.WithStepName("gatehouse.session-tool-call-project-note-remove"))
		return err, removed
	}
}

func (runtime *SessionEventReplyRuntime) projectNoteUpdate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) ProjectNoteUpdate {
	return func(id, title, description, body string, sensitive bool) (error, ProjectNote) {
		note, err := dbos.RunAsStep(ctx, func(step context.Context) (ProjectNote, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectNoteEdit)
			if err != nil {
				err = fmt.Errorf("update project note: %w", err)
				return ProjectNote{}, err
			}
			err, updated := runtime.store.ProjectNoteDetailsSetAs(step, model.ProjectNoteRef{Project: *project, Id: id}, principal, database.NoteAuthor{Agent: &agent}, sensitive, &title, &description, &body)
			if err != nil || updated == nil {
				if err == nil {
					err = fmt.Errorf("update project note: unavailable")
				}
				return ProjectNote{}, err
			}
			return ProjectNote{ID: updated.Note.Ref.Id, Title: updated.Note.Title, Description: updated.Note.Description, Sensitive: updated.Note.Sensitive, AuthorID: noteAuthorID(updated.Note.AuthorPrincipal, updated.Note.AuthorAgent, updated.Note.AuthorGateway), CreatedAt: updated.Note.CreatedAt, Revision: updated.Note.Revision}, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-project-note-update"))
		return err, note
	}
}

func (runtime *SessionEventReplyRuntime) projectInfoSet(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) ProjectInfoSet {
	return func(name, description string) (error, ProjectInfo) {
		updated, err := dbos.RunAsStep(ctx, func(step context.Context) (model.Project, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectEdit)
			if err != nil {
				return model.Project{}, fmt.Errorf("update project details: %w", err)
			}
			err, updated := runtime.store.ProjectDetailsSet(step, *project, principal, &name, &description)
			if err != nil {
				return model.Project{}, err
			}
			if updated == nil {
				return model.Project{}, fmt.Errorf("update project details: project is unavailable")
			}
			return *updated, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-project-info-set"))
		return err, ProjectInfo{Name: updated.Name, Description: updated.Description, CreatedAt: updated.CreatedAt}
	}
}

func (runtime *SessionEventReplyRuntime) projectRecords(ctx context.Context, session model.SessionRef, principal model.PrincipalRef) *ProjectRecords {
	return &ProjectRecords{
		Schemas: func() (error, []ProjectRecordSchema) {
			project, err := runtime.sessionProjectGet(ctx, session, principal)
			if err != nil {
				return err, nil
			}
			err, schemas := runtime.store.ProjectRecordSchemasGet(ctx, *project, principal)
			if err != nil {
				return err, nil
			}
			result := make([]ProjectRecordSchema, 0, len(schemas))
			for _, schema := range schemas {
				result = append(result, projectRecordSchemaFromModel(schema))
			}
			return nil, result
		},
		Schema: func(id string) (error, *ProjectRecordSchema) {
			project, err := runtime.sessionProjectGet(ctx, session, principal)
			if err != nil {
				return err, nil
			}
			err, schema := runtime.store.ProjectRecordSchemaGet(ctx, model.ProjectRecordSchemaRef{Project: *project, Id: id}, principal)
			if err != nil || schema == nil {
				return err, nil
			}
			result := projectRecordSchemaFromModel(*schema)
			return nil, &result
		},
		Attributes: func(schemaID string) (error, []ProjectRecordAttribute) {
			project, err := runtime.sessionProjectGet(ctx, session, principal)
			if err != nil {
				return err, nil
			}
			err, attributes := runtime.store.ProjectRecordAttributesGet(ctx, model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}, principal)
			if err != nil {
				return err, nil
			}
			result := make([]ProjectRecordAttribute, 0, len(attributes))
			for _, attribute := range attributes {
				result = append(result, projectRecordAttributeFromModel(attribute))
			}
			return nil, result
		},
		Attribute: func(schemaID, id string) (error, *ProjectRecordAttribute) {
			project, err := runtime.sessionProjectGet(ctx, session, principal)
			if err != nil {
				return err, nil
			}
			err, attribute := runtime.store.ProjectRecordAttributeGet(ctx, model.ProjectRecordAttributeRef{Schema: model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}, Id: id}, principal)
			if err != nil || attribute == nil {
				return err, nil
			}
			result := projectRecordAttributeFromModel(*attribute)
			return nil, &result
		},
		Records: func(schemaID, cursor string) (error, []ProjectRecord) {
			project, err := runtime.sessionProjectGet(ctx, session, principal)
			if err != nil {
				return err, nil
			}
			err, records := runtime.store.ProjectRecordsGet(ctx, model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}, principal, 100, cursor)
			if err != nil {
				return err, nil
			}
			result := make([]ProjectRecord, 0, len(records))
			for _, record := range records {
				result = append(result, projectRecordFromModel(record))
			}
			return nil, result
		},
		Record: func(schemaID, id string) (error, *ProjectRecord) {
			project, err := runtime.sessionProjectGet(ctx, session, principal)
			if err != nil {
				return err, nil
			}
			err, record := runtime.store.ProjectRecordGet(ctx, model.ProjectRecordRef{Schema: model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}, Id: id}, principal)
			if err != nil || record == nil {
				return err, nil
			}
			result := projectRecordFromModel(*record)
			return nil, &result
		},
		Values: func(schemaID, recordID, cursor string) (error, []ProjectRecordValue) {
			project, err := runtime.sessionProjectGet(ctx, session, principal)
			if err != nil {
				return err, nil
			}
			err, values := runtime.store.ProjectRecordValuesGet(ctx, model.ProjectRecordRef{Schema: model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}, Id: recordID}, principal, 100, cursor)
			if err != nil {
				return err, nil
			}
			result := make([]ProjectRecordValue, 0, len(values))
			for _, value := range values {
				result = append(result, projectRecordValueFromModel(value))
			}
			return nil, result
		},
	}
}

func (runtime *SessionEventReplyRuntime) projectRecordSchemaCreate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) ProjectRecordSchemaCreate {
	return func(name, label, description string) (error, ProjectRecordSchema) {
		result, err := dbos.RunAsStep(ctx, func(step context.Context) (ProjectRecordSchema, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectRecordSchemaCreate)
			if err != nil {
				return ProjectRecordSchema{}, err
			}
			id, err := typed_id.New(typed_id.ProjectRecordSchema)
			if err != nil {
				return ProjectRecordSchema{}, err
			}
			err, schema := runtime.store.ProjectRecordSchemaCreate(step, model.ProjectRecordSchema{Ref: model.ProjectRecordSchemaRef{Project: *project, Id: id}, Name: name, Label: label, Description: description}, principal, database.ProjectRecordAuthor{Agent: &agent})
			return projectRecordSchemaFromModel(schema), err
		}, dbos.WithStepName("gatehouse.session-tool-call-project-record-schema-create"))
		return err, result
	}
}

func (runtime *SessionEventReplyRuntime) projectRecordSchemaUpdate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) ProjectRecordSchemaUpdate {
	return func(id, label, description string) (error, *ProjectRecordSchema) {
		result, err := dbos.RunAsStep(ctx, func(step context.Context) (*ProjectRecordSchema, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectRecordSchemaEdit)
			if err != nil {
				return nil, err
			}
			err, schema := runtime.store.ProjectRecordSchemaDetailsSetAs(step, model.ProjectRecordSchemaRef{Project: *project, Id: id}, principal, database.ProjectRecordAuthor{Agent: &agent}, label, description)
			if err != nil || schema == nil {
				return nil, err
			}
			result := projectRecordSchemaFromModel(*schema)
			return &result, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-project-record-schema-update"))
		return err, result
	}
}

func (runtime *SessionEventReplyRuntime) projectRecordSchemaRemove(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) ProjectRecordSchemaRemove {
	return func(id string) (error, bool) {
		removed, err := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectRecordSchemaRemove)
			if err != nil {
				return false, err
			}
			err, removed := runtime.store.ProjectRecordSchemaRemove(step, model.ProjectRecordSchemaRef{Project: *project, Id: id}, principal)
			return removed, err
		}, dbos.WithStepName("gatehouse.session-tool-call-project-record-schema-remove"))
		return err, removed
	}
}

func (runtime *SessionEventReplyRuntime) projectRecordAttributeCreate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) ProjectRecordAttributeCreate {
	return func(schemaID string, input ProjectRecordAttributeInput) (error, ProjectRecordAttribute) {
		result, err := dbos.RunAsStep(ctx, func(step context.Context) (ProjectRecordAttribute, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectRecordSchemaEdit)
			if err != nil {
				return ProjectRecordAttribute{}, err
			}
			id, err := typed_id.New(typed_id.ProjectRecordAttribute)
			if err != nil {
				return ProjectRecordAttribute{}, err
			}
			schema := model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}
			attribute := model.ProjectRecordAttribute{Ref: model.ProjectRecordAttributeRef{Schema: schema, Id: id}, Name: input.Name, Label: input.Label, Description: input.Description, Type: input.Type, Cardinality: input.Cardinality, Uniqueness: input.Uniqueness, Display: input.Display, DisplayOrder: input.DisplayOrder}
			if input.TargetSchema != nil {
				attribute.TargetSchema = &model.ProjectRecordSchemaRef{Project: *project, Id: *input.TargetSchema}
			}
			err, stored := runtime.store.ProjectRecordAttributeCreate(step, attribute, principal, database.ProjectRecordAuthor{Agent: &agent})
			return projectRecordAttributeFromModel(stored), err
		}, dbos.WithStepName("gatehouse.session-tool-call-project-record-attribute-create"))
		return err, result
	}
}

func (runtime *SessionEventReplyRuntime) projectRecordAttributeUpdate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) ProjectRecordAttributeUpdate {
	return func(schemaID, id string, input ProjectRecordAttributeUpdateInput) (error, *ProjectRecordAttribute) {
		result, err := dbos.RunAsStep(ctx, func(step context.Context) (*ProjectRecordAttribute, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectRecordSchemaEdit)
			if err != nil {
				return nil, err
			}
			schema := model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}
			err, current := runtime.store.ProjectRecordAttributeGet(step, model.ProjectRecordAttributeRef{Schema: schema, Id: id}, principal)
			if err != nil || current == nil {
				return nil, err
			}
			current.Label, current.Description, current.Type, current.Cardinality, current.Uniqueness, current.Display, current.DisplayOrder = input.Label, input.Description, input.Type, input.Cardinality, input.Uniqueness, input.Display, input.DisplayOrder
			current.TargetSchema = nil
			if input.TargetSchema != nil {
				current.TargetSchema = &model.ProjectRecordSchemaRef{Project: *project, Id: *input.TargetSchema}
			}
			err, stored := runtime.store.ProjectRecordAttributeSetAs(step, *current, principal, database.ProjectRecordAuthor{Agent: &agent})
			if err != nil || stored == nil {
				return nil, err
			}
			value := projectRecordAttributeFromModel(*stored)
			return &value, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-project-record-attribute-update"))
		return err, result
	}
}

func (runtime *SessionEventReplyRuntime) projectRecordAttributeRemove(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) ProjectRecordAttributeRemove {
	return func(schemaID, id string) (error, bool) {
		removed, err := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectRecordSchemaRemove)
			if err != nil {
				return false, err
			}
			err, removed := runtime.store.ProjectRecordAttributeRemove(step, model.ProjectRecordAttributeRef{Schema: model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}, Id: id}, principal)
			return removed, err
		}, dbos.WithStepName("gatehouse.session-tool-call-project-record-attribute-remove"))
		return err, removed
	}
}

func (runtime *SessionEventReplyRuntime) projectRecordCreate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) ProjectRecordCreate {
	return func(schemaID string, values []ProjectRecordValueInput) (error, ProjectRecord, []ProjectRecordValue) {
		result, err := dbos.RunAsStep(ctx, func(step context.Context) (struct {
			Record ProjectRecord
			Values []ProjectRecordValue
		}, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectRecordCreate)
			if err != nil {
				return struct {
					Record ProjectRecord
					Values []ProjectRecordValue
				}{}, err
			}
			schema := model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}
			err, attributes := runtime.store.ProjectRecordAttributesGet(step, schema, principal)
			if err != nil {
				return struct {
					Record ProjectRecord
					Values []ProjectRecordValue
				}{}, err
			}
			byName := make(map[string]model.ProjectRecordAttributeRef, len(attributes))
			for _, attribute := range attributes {
				byName[attribute.Name] = attribute.Ref
			}
			creates := make([]database.ProjectRecordValueCreate, 0, len(values))
			for _, value := range values {
				attribute, ok := byName[value.Attribute]
				if !ok {
					return struct {
						Record ProjectRecord
						Values []ProjectRecordValue
					}{}, fmt.Errorf("record attribute is unavailable")
				}
				creates = append(creates, database.ProjectRecordValueCreate{Attribute: attribute, Value: value.Value, Sensitive: value.Sensitive})
			}
			id, err := typed_id.New(typed_id.ProjectRecord)
			if err != nil {
				return struct {
					Record ProjectRecord
					Values []ProjectRecordValue
				}{}, err
			}
			err, record, created := runtime.store.ProjectRecordCreate(step, model.ProjectRecord{Ref: model.ProjectRecordRef{Schema: schema, Id: id}}, principal, database.ProjectRecordAuthor{Agent: &agent}, creates)
			if err != nil {
				return struct {
					Record ProjectRecord
					Values []ProjectRecordValue
				}{}, err
			}
			result := struct {
				Record ProjectRecord
				Values []ProjectRecordValue
			}{Record: projectRecordFromModel(record), Values: make([]ProjectRecordValue, 0, len(created))}
			for _, value := range created {
				result.Values = append(result.Values, projectRecordValueFromModel(value))
			}
			return result, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-project-record-create"))
		return err, result.Record, result.Values
	}
}

func (runtime *SessionEventReplyRuntime) projectRecordRemove(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) ProjectRecordRemove {
	return func(schemaID, id string) (error, bool) {
		removed, err := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectRecordRemove)
			if err != nil {
				return false, err
			}
			err, removed := runtime.store.ProjectRecordRemove(step, model.ProjectRecordRef{Schema: model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}, Id: id}, principal)
			return removed, err
		}, dbos.WithStepName("gatehouse.session-tool-call-project-record-remove"))
		return err, removed
	}
}

func (runtime *SessionEventReplyRuntime) projectRecordValuesMutate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) ProjectRecordValuesMutate {
	return func(schemaID, recordID string, input ProjectRecordValuesMutation) (error, ProjectRecordValuesMutationResult) {
		result, err := dbos.RunAsStep(ctx, func(step context.Context) (ProjectRecordValuesMutationResult, error) {
			project, err := runtime.sessionProjectActionGet(step, session, principal, authz.ProjectRecordEdit)
			if err != nil {
				return ProjectRecordValuesMutationResult{}, err
			}
			schema := model.ProjectRecordSchemaRef{Project: *project, Id: schemaID}
			record := model.ProjectRecordRef{Schema: schema, Id: recordID}
			err, attributes := runtime.store.ProjectRecordAttributesGet(step, schema, principal)
			if err != nil {
				return ProjectRecordValuesMutationResult{}, err
			}
			byName := make(map[string]model.ProjectRecordAttributeRef, len(attributes))
			for _, attribute := range attributes {
				byName[attribute.Name] = attribute.Ref
			}
			mutation := database.ProjectRecordValuesMutation{Delete: input.Remove}
			for _, value := range input.Create {
				attribute, ok := byName[value.Attribute]
				if !ok {
					return ProjectRecordValuesMutationResult{}, fmt.Errorf("record attribute is unavailable")
				}
				mutation.Create = append(mutation.Create, database.ProjectRecordValueCreate{Attribute: attribute, Value: value.Value, Sensitive: value.Sensitive})
			}
			for _, value := range input.Update {
				sensitive := value.Sensitive
				mutation.Update = append(mutation.Update, database.ProjectRecordValueUpdate{ID: value.ID, Value: value.Value, Sensitive: &sensitive})
			}
			err, stored := runtime.store.ProjectRecordValuesMutate(step, record, principal, database.ProjectRecordAuthor{Agent: &agent}, mutation)
			if err != nil {
				return ProjectRecordValuesMutationResult{}, err
			}
			result := ProjectRecordValuesMutationResult{Removed: stored.Removed, Created: make([]ProjectRecordValue, 0, len(stored.Created))}
			for _, value := range stored.Created {
				result.Created = append(result.Created, projectRecordValueFromModel(value))
			}
			return result, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-project-record-values-mutate"))
		return err, result
	}
}

func projectRecordAuthorID(principal *model.PrincipalRef, agent *model.WorkspaceAgentRef) string {
	if agent != nil {
		return agent.Id
	}
	if principal != nil {
		return principal.Id
	}
	return ""
}
func projectRecordSchemaFromModel(schema model.ProjectRecordSchema) ProjectRecordSchema {
	return ProjectRecordSchema{ID: schema.Ref.Id, Name: schema.Name, Label: schema.Label, Description: schema.Description, AuthorID: projectRecordAuthorID(schema.AuthorPrincipal, schema.AuthorAgent), CreatedAt: schema.CreatedAt}
}
func projectRecordAttributeFromModel(attribute model.ProjectRecordAttribute) ProjectRecordAttribute {
	result := ProjectRecordAttribute{ID: attribute.Ref.Id, Name: attribute.Name, Label: attribute.Label, Description: attribute.Description, Type: attribute.Type, Cardinality: attribute.Cardinality, Uniqueness: attribute.Uniqueness, Display: attribute.Display, DisplayOrder: attribute.DisplayOrder, AuthorID: projectRecordAuthorID(attribute.AuthorPrincipal, attribute.AuthorAgent), CreatedAt: attribute.CreatedAt}
	if attribute.TargetSchema != nil {
		target := attribute.TargetSchema.Id
		result.TargetSchema = &target
	}
	return result
}
func projectRecordFromModel(record model.ProjectRecord) ProjectRecord {
	return ProjectRecord{ID: record.Ref.Id, AuthorID: projectRecordAuthorID(record.AuthorPrincipal, record.AuthorAgent), CreatedAt: record.CreatedAt}
}
func projectRecordValueFromModel(value model.ProjectRecordValue) ProjectRecordValue {
	return ProjectRecordValue{ID: value.Ref.Id, Attribute: value.Attribute.Id, Value: value.Value, Sensitive: value.Sensitive, AuthorID: projectRecordAuthorID(value.AuthorPrincipal, value.AuthorAgent), CreatedAt: value.CreatedAt}
}

func (runtime *SessionEventReplyRuntime) awaitApproval(ctx dbos.Context, tool model.SessionEvent, agent model.WorkspaceAgentRef, description string) error {
	payload, err := database.SessionEventPayloadFrom(model.ApprovalRequestPayload{Description: description})
	if err != nil {
		return err
	}
	err, request := runtime.persistAgentEvent(ctx, model.SessionEvent{
		Ref: model.SessionEventRef{Session: tool.Ref.Session}, Parent: &tool.Ref, Kind: model.SessionEventKindApprovalRequest, AuthorAgent: &agent, Payload: payload,
	})
	if err != nil {
		return err
	}
	handle, err := dbos.RunWorkflow(ctx, runtime.approval, SessionApprovalInput{Request: request},
		dbos.WithRunInstance(runtime),
		dbos.WithWorkflowID(sessionApprovalWorkflowID(request)),
	)
	if err != nil {
		return fmt.Errorf("start approval %q: %w", request.Id, err)
	}
	outcome, err := handle.GetResult()
	if err != nil {
		return fmt.Errorf("await approval %q: %w", request.Id, err)
	}
	if outcome == ApprovalGranted {
		return nil
	}
	if outcome == ApprovalRejected {
		return lisp.Errorf("approval rejected")
	}
	return fmt.Errorf("approval %q returned invalid outcome %q", request.Id, outcome)
}

func (runtime *SessionEventReplyRuntime) approval(ctx dbos.Context, input SessionApprovalInput) (ApprovalOutcome, error) {
	decision, err := dbos.Recv[model.SessionEventRef](ctx, "response", sessionApprovalWait)
	if err != nil {
		return "", err
	}
	return dbos.RunAsStep(ctx, func(step context.Context) (ApprovalOutcome, error) {
		err, event := runtime.store.SessionEventGet(step, decision)
		if err != nil {
			return "", err
		}
		if event == nil || event.Parent == nil || *event.Parent != input.Request || event.AuthorPrincipal == nil {
			return "", fmt.Errorf("approval response is invalid")
		}
		switch event.Kind {
		case model.SessionEventKindApprovalSuccess:
			return ApprovalGranted, nil
		case model.SessionEventKindApprovalFailure:
			return ApprovalRejected, nil
		default:
			return "", fmt.Errorf("approval response has invalid kind %q", event.Kind)
		}
	}, dbos.WithStepName("gatehouse.session-approval-response"))
}

func (runtime *SessionEventReplyRuntime) awaitInput(ctx dbos.Context, tool model.SessionEvent, agent model.WorkspaceAgentRef, form inputform.Form) (error, string) {
	if err := form.Validate(); err != nil {
		return err, ""
	}
	payload, err := database.SessionEventPayloadFrom(model.InputRequestPayload{Description: form.Title, Form: form})
	if err != nil {
		return err, ""
	}
	err, request := runtime.persistAgentEvent(ctx, model.SessionEvent{
		Ref: model.SessionEventRef{Session: tool.Ref.Session}, Parent: &tool.Ref, Kind: model.SessionEventKindInputRequest, AuthorAgent: &agent,
		Payload: payload,
	})
	if err != nil {
		return err, ""
	}
	handle, err := dbos.RunWorkflow(ctx, runtime.input, SessionInputWaitInput{Request: request},
		dbos.WithRunInstance(runtime), dbos.WithWorkflowID(sessionInputWorkflowID(request)))
	if err != nil {
		return fmt.Errorf("start input %q: %w", request.Id, err), ""
	}
	outcome, err := handle.GetResult()
	if err != nil {
		return fmt.Errorf("await input %q: %w", request.Id, err), ""
	}
	if outcome.Failure != "" {
		return lisp.Errorf("input failed: %s", outcome.Failure), ""
	}
	if outcome.Result == "" {
		return fmt.Errorf("input %q returned an empty result", request.Id), ""
	}
	return nil, outcome.Result
}

func (runtime *SessionEventReplyRuntime) input(ctx dbos.Context, input SessionInputWaitInput) (InputOutcome, error) {
	response, err := dbos.Recv[model.SessionEventRef](ctx, "response", sessionApprovalWait)
	if err != nil {
		return InputOutcome{}, err
	}
	return dbos.RunAsStep(ctx, func(step context.Context) (InputOutcome, error) {
		err, event := runtime.store.SessionEventGet(step, response)
		if err != nil {
			return InputOutcome{}, err
		}
		if event == nil || event.Parent == nil || *event.Parent != input.Request || event.AuthorPrincipal == nil {
			return InputOutcome{}, fmt.Errorf("input response is invalid")
		}
		switch event.Kind {
		case model.SessionEventKindInputSuccess:
			err, result := runtime.store.SessionInputResponseResultGet(step, response)
			return InputOutcome{Result: result}, err
		case model.SessionEventKindInputFailure:
			code, ok := event.Payload["code"].(string)
			if !ok || code == "" {
				return InputOutcome{}, fmt.Errorf("input response has an invalid failure code")
			}
			return InputOutcome{Failure: code}, nil
		default:
			return InputOutcome{}, fmt.Errorf("input response has invalid kind %q", event.Kind)
		}
	}, dbos.WithStepName("gatehouse.session-input-response"))
}

func (runtime *SessionEventReplyRuntime) runLispCalls(ctx dbos.Context, parent model.SessionEventRef, selected *database.WorkspaceAgentModel, principal model.PrincipalRef, round, offset int, calls []openAICompatibleToolCall) ([]model.SessionEventRef, error) {
	agent := selected.Ref
	inputs := make([]SessionToolCallInput, len(calls))
	seen := make(map[string]bool, len(calls))
	for index, call := range calls {
		code, reason, err := openAICompatibleLispArguments(call)
		if err != nil {
			return nil, err
		}
		if seen[call.ID] {
			return nil, fmt.Errorf("OpenAI-compatible completion requested a duplicate tool call ID")
		}
		seen[call.ID] = true
		batch := round
		position := offset + index
		payload, err := database.SessionEventPayloadFrom(model.ToolRequestPayload{Name: "lisp", CallId: call.ID, Code: code, Reason: reason, Batch: &batch, Position: &position})
		if err != nil {
			return nil, fmt.Errorf("encode tool request payload %q: %w", call.ID, err)
		}
		request := model.SessionEvent{
			Ref:         model.SessionEventRef{Session: parent.Session},
			Parent:      &parent,
			Kind:        model.SessionEventKindToolRequest,
			AuthorAgent: &agent,
			Payload:     payload,
		}
		err, stored := runtime.persistAgentEvent(ctx, request)
		if err != nil {
			return nil, err
		}
		request.Ref = stored
		inputs[index] = SessionToolCallInput{Request: request.Ref}
	}
	handles := make([]dbos.WorkflowHandle[model.SessionEventRef], len(inputs))
	for index, input := range inputs {
		handle, err := dbos.RunWorkflow(ctx, runtime.toolCall, input,
			dbos.WithRunInstance(runtime),
			dbos.WithWorkflowID(sessionToolCallWorkflowID(input.Request)),
			dbos.WithQueue(runtime.toolQueue),
		)
		if err != nil {
			return nil, fmt.Errorf("start session tool call %q: %w", calls[index].ID, err)
		}
		handles[index] = handle
	}
	outputs := make([]model.SessionEventRef, len(handles))
	for index, handle := range handles {
		output, err := handle.GetResult()
		if err != nil {
			return nil, fmt.Errorf("await session tool call %q: %w", calls[index].ID, err)
		}
		outputs[index] = output
	}
	return outputs, nil
}

func openAICompatibleLispArguments(call openAICompatibleToolCall) (string, string, error) {
	var arguments struct {
		Code   string `json:"code"`
		Reason string `json:"reason"`
	}
	if call.Type != "function" || call.Function.Name != "lisp" || call.ID == "" || json.Unmarshal([]byte(call.Function.Arguments), &arguments) != nil || strings.TrimSpace(arguments.Code) == "" || strings.TrimSpace(arguments.Reason) == "" {
		return "", "", fmt.Errorf("OpenAI-compatible completion requested an unsupported tool call")
	}
	return arguments.Code, arguments.Reason, nil
}

func openAIFinalReply(contents string) (agentFinalReply, error) {
	var reply struct {
		Text        *string   `json:"text"`
		Attachments *[]string `json:"attachments"`
	}
	decoder := json.NewDecoder(strings.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&reply); err != nil || decoder.Decode(&struct{}{}) != io.EOF || reply.Text == nil || reply.Attachments == nil {
		return agentFinalReply{}, fmt.Errorf("OpenAI-compatible completion returned an invalid structured reply")
	}
	return agentFinalReply{Text: *reply.Text, Attachments: *reply.Attachments}, nil
}

func logInvalidOpenAIFinalReply(providerID string, parent model.SessionEventRef, contents string) {
	fmt.Fprintf(os.Stderr, "gatehouse: invalid OpenAI final reply provider=%q parent_event=%q content=%q\n", providerID, parent.Id, contents)
}

func openAIResponsesFinalReply(outputs []openAIResponsesOutput) (agentFinalReply, error) {
	for _, output := range outputs {
		if output.Type != "message" || output.Status != "completed" {
			continue
		}
		for _, content := range output.Content {
			if content.Type != "output_text" {
				continue
			}
			final, err := openAIFinalReply(content.Text)
			if err == nil {
				return final, nil
			}
		}
	}
	return agentFinalReply{}, fmt.Errorf("OpenAI Responses returned no valid final output text")
}

func logInvalidOpenAIResponsesFinalReply(providerID string, parent model.SessionEventRef, input, outputs []json.RawMessage) {
	encodedInput, _ := json.Marshal(input)
	encodedOutput, _ := json.Marshal(outputs)
	fmt.Fprintf(os.Stderr, "gatehouse: invalid OpenAI Responses final reply provider=%q parent_event=%q input=%s output=%s\n", providerID, parent.Id, encodedInput, encodedOutput)
}

//go:embed system_prompt_0_role.tmpl
var systemPromptRoleSource string

var systemPromptRole = template.Must(template.New("system_prompt_0_role").Parse(systemPromptRoleSource))

//go:embed system_prompt_1_tools.txt
var systemPromptTools string

//go:embed system_prompt_2_session.txt
var systemPromptSession string

//go:embed system_prompt_2_project.txt
var systemPromptProject string

//go:embed system_prompt_2_inputs.txt
var systemPromptInputs string

//go:embed system_prompt_3_workflow.txt
var systemPromptWorkflow string

func openAISystemPromptFor(selected *database.WorkspaceAgentModel, projectChat bool) string {
	var role strings.Builder
	if err := systemPromptRole.Execute(&role, struct{ Handle string }{Handle: "@" + selected.Alias}); err != nil {
		panic(fmt.Sprintf("render system prompt role: %v", err))
	}
	prompt := role.String() + "\n" + systemPromptTools + "\n" + systemPromptSession
	if projectChat {
		prompt += "\n" + systemPromptProject
	}
	prompt += "\n" + systemPromptInputs + "\n" + systemPromptWorkflow
	if selected.SystemPrompt == nil || *selected.SystemPrompt == "" {
		return prompt
	}
	return prompt + "\n\n" + *selected.SystemPrompt
}

func agentPreludeFor(selected *database.WorkspaceAgentModel, projectChat bool) string {
	if selected.Prelude != nil {
		return *selected.Prelude
	}
	if !projectChat {
		return agentSessionPrelude
	}
	return agentPrelude
}

func openAIRequestMessages(selected *database.WorkspaceAgentModel, messages []openAICompatibleMessage, projectChat bool) []openAICompatibleMessage {
	prompt := openAISystemPromptFor(selected, projectChat)
	if prompt == "" {
		return messages
	}
	return append([]openAICompatibleMessage{{Role: "system", Content: prompt}}, messages...)
}

func openAICompatibleReasoningEffort(parameters string) (error, string) {
	var configured struct {
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if err := json.Unmarshal([]byte(parameters), &configured); err != nil {
		return fmt.Errorf("decode OpenAI-compatible model parameters: %w", err), ""
	}
	if configured.ReasoningEffort == "" {
		return nil, ""
	}
	for _, effort := range []string{"none", "low", "medium", "high", "xhigh", "max"} {
		if configured.ReasoningEffort == effort {
			return nil, configured.ReasoningEffort
		}
	}
	return fmt.Errorf("OpenAI-compatible reasoning_effort %q is invalid", configured.ReasoningEffort), ""
}

func (runtime *SessionEventReplyRuntime) persistAgentEvent(ctx dbos.Context, event model.SessionEvent) (error, model.SessionEventRef) {
	reference, err := dbos.RunAsTransaction(ctx, runtime.dataSource, func(step context.Context, transaction dbos.Tx) (model.SessionEventRef, error) {
		id, err := typed_id.New(typed_id.SessionEvent)
		if err != nil {
			return model.SessionEventRef{}, fmt.Errorf("generate session event ID: %w", err)
		}
		event.Ref.Id = id
		err, _ = runtime.store.SessionEventCreateInTransaction(step, transaction, event)
		return event.Ref, err
	}, dbos.WithStepName("gatehouse.session-event-agent-persist"))
	return err, reference
}

func (runtime *SessionEventReplyRuntime) replyCancellationRequested(ctx context.Context, parent model.SessionEventRef) (error, *model.SessionEvent) {
	return runtime.store.SessionEventChildGet(ctx, parent, model.SessionEventKindCancelRequest)
}

// cancelReplyWorkflowTree durably cancels the reply and every DBOS child,
// including parallel tool calls and their input or approval waits.
func (runtime *SessionEventReplyRuntime) cancelReplyWorkflowTree(ctx dbos.Context, request model.SessionEventRef) error {
	if err := dbos.CancelWorkflow(ctx, sessionEventReplyWorkflowID(request), dbos.WithCancelChildren()); err != nil {
		return fmt.Errorf("cancel session event reply workflow %q: %w", request.Id, err)
	}
	return nil
}

func (runtime *SessionEventReplyRuntime) replyCancellationCheck(ctx dbos.Context, parent model.SessionEventRef) error {
	err, cancelled := runtime.replyCancellationRequested(ctx, parent)
	if err != nil {
		return err
	}
	if cancelled == nil {
		return nil
	}
	return sessionReplyCancelled{}
}

func (runtime *SessionEventReplyRuntime) thinkingStart(ctx dbos.Context, parent model.SessionEventRef, agent model.WorkspaceAgentRef, turn int) (error, model.SessionEvent) {
	payload, err := database.SessionEventPayloadFrom(model.ThinkingRequestPayload{Turn: turn})
	if err != nil {
		return err, model.SessionEvent{}
	}
	event := model.SessionEvent{
		Ref: model.SessionEventRef{Session: parent.Session}, Parent: &parent, Kind: model.SessionEventKindThinkingRequest, AuthorAgent: &agent,
		Payload: payload,
	}
	err, reference := runtime.persistAgentEvent(ctx, event)
	if err != nil {
		return err, model.SessionEvent{}
	}
	createdAt, err := typed_id.Timestamp(typed_id.SessionEvent, reference.Id)
	if err != nil {
		return err, model.SessionEvent{}
	}
	event.Ref = reference
	event.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	return nil, event
}

func (runtime *SessionEventReplyRuntime) thinkingDelay(ctx dbos.Context, started model.SessionEvent, until time.Time) error {
	untilValue := until.UTC().Format(time.RFC3339Nano)
	payload, err := database.SessionEventPayloadFrom(model.ThinkingUpdatePayload{Reason: "rate_limit", Until: &untilValue})
	if err != nil {
		return err
	}
	event := model.SessionEvent{
		Ref: model.SessionEventRef{Session: started.Ref.Session}, Parent: &started.Ref, Kind: model.SessionEventKindThinkingUpdate, AuthorAgent: started.AuthorAgent,
		Payload: payload,
	}
	err, _ = runtime.persistAgentEvent(ctx, event)
	return err
}

func (runtime *SessionEventReplyRuntime) thinkingFinish(ctx dbos.Context, started model.SessionEvent, kind string, completionErr error) error {
	return runtime.thinkingFinishWithMetrics(ctx, started, kind, completionErr, nil)
}

func (runtime *SessionEventReplyRuntime) thinkingFinishWithMetrics(ctx dbos.Context, started model.SessionEvent, kind string, completionErr error, metrics *model.SessionEventMetrics) error {
	var payload map[string]interface{}
	var err error
	if kind == model.SessionEventKindThinkingSuccess {
		payload, err = database.SessionEventPayloadFrom(model.ThinkingSuccessPayload{})
	} else {
		message := ""
		if completionErr != nil {
			message = completionErr.Error()
		}
		payload, err = database.SessionEventPayloadFrom(model.ThinkingFailurePayload{Code: failureEventCode(completionErr), Message: optionalString(message)})
	}
	if err != nil {
		return fmt.Errorf("encode thinking terminal payload: %w", err)
	}
	event := model.SessionEvent{
		Ref: model.SessionEventRef{Session: started.Ref.Session}, Parent: &started.Ref, Kind: kind, AuthorAgent: started.AuthorAgent,
		Payload: payload, Metrics: metrics, CreatedAt: eventTerminalCreatedAt(started.CreatedAt),
	}
	err, _ = runtime.persistAgentEvent(ctx, event)
	if completionErr != nil && err != nil {
		return errors.Join(completionErr, err)
	}
	if completionErr != nil {
		return completionErr
	}
	return err
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func openAICompatibleLispTool() openAICompatibleTool {
	tool := openAICompatibleTool{Type: "function"}
	tool.Function.Name = "lisp"
	tool.Function.Description = "Evaluate Lisp using available native modules. Explain the purpose in reason. Use (help/env) and (help/search) to discover this Lisp dialect; it is not Common Lisp. Correct and retry failed evaluations when possible."
	tool.Function.Parameters = map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"code", "reason"},
		"properties": map[string]any{
			"code":   map[string]any{"type": "string", "description": "Lisp expression to evaluate."},
			"reason": map[string]any{"type": "string", "description": "Why this evaluation is needed."},
		},
	}
	return tool
}

func openAIFinalReplyResponseFormat() *openAIResponseFormat {
	return &openAIResponseFormat{Type: "json_schema", JSONSchema: openAIJSONSchemaFormat{Name: "gatehouse_agent_reply", Strict: true, Schema: openAIFinalReplySchema()}}
}

func openAIResponsesFinalReplyText() *openAIResponsesText {
	return &openAIResponsesText{Format: openAIJSONSchemaFormat{Type: "json_schema", Name: "gatehouse_agent_reply", Strict: true, Schema: openAIFinalReplySchema()}}
}

func openAIFinalReplySchema() map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"text", "attachments"},
		"properties": map[string]any{
			"text":        map[string]any{"type": "string", "description": "Final user-facing text."},
			"attachments": map[string]any{"type": "array", "description": "Session file IDs to attach.", "items": map[string]any{"type": "string"}},
		},
	}
}

func openAIResponsesLispTool() openAIResponsesTool {
	compatible := openAICompatibleLispTool()
	return openAIResponsesTool{Type: "function", Name: compatible.Function.Name, Description: compatible.Function.Description, Parameters: compatible.Function.Parameters, Strict: true}
}

func projectTaskValue(task model.ProjectTask) Task {
	return Task{ID: task.Ref.Id, Title: task.Title, Sensitive: task.Sensitive, Status: task.Status,
		CreatorID: noteAuthorID(task.CreatorPrincipal, task.CreatorAgent, task.CreatorGateway), UpdaterID: noteAuthorID(task.UpdaterPrincipal, task.UpdaterAgent, task.UpdaterGateway), CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt}
}

func sessionTaskValue(task model.SessionTask) Task {
	return Task{ID: task.Ref.Id, Title: task.Title, Sensitive: task.Sensitive, Status: task.Status,
		CreatorID: noteAuthorID(task.CreatorPrincipal, task.CreatorAgent, task.CreatorGateway), UpdaterID: noteAuthorID(task.UpdaterPrincipal, task.UpdaterAgent, task.UpdaterGateway), CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt}
}

func noteAuthorMetadata(principal *model.PrincipalRef, principalName *string, agent *model.WorkspaceAgentRef, agentLabel *string, gateway *model.GatewayRef) (string, *string) {
	if principal != nil {
		return principal.Id, principalName
	}
	if agent != nil {
		return agent.Id, agentLabel
	}
	if gateway != nil {
		return gateway.Id, nil
	}
	return "", nil
}

func noteAuthorID(principal *model.PrincipalRef, agent *model.WorkspaceAgentRef, gateway *model.GatewayRef) string {
	if principal != nil {
		return principal.Id
	}
	if agent != nil {
		return agent.Id
	}
	if gateway != nil {
		return gateway.Id
	}
	return ""
}

func openAICompatibleMessages(events []model.SessionEvent) (error, []openAICompatibleMessage) {
	messages := make([]openAICompatibleMessage, 0, len(events))
	calls := map[string]openAICompatibleToolCall{}
	type batch struct {
		calls []struct {
			position int
			call     openAICompatibleToolCall
			event    model.SessionEvent
		}
		emitted bool
	}
	batches := map[string]*batch{}
	callBatches := map[string]string{}
	for _, event := range events {
		if event.Kind != model.SessionEventKindToolRequest || event.AuthorAgent == nil {
			continue
		}
		call, err := openAICompatibleStoredToolCall(event)
		if err != nil {
			return err, nil
		}
		calls[event.Ref.Id] = call
		batchNumber, batchOK := event.Payload["batch"].(float64)
		position, positionOK := event.Payload["position"].(float64)
		if batchOK && positionOK && event.Parent != nil {
			key := event.Parent.Id + "\x00" + fmt.Sprintf("%.0f", batchNumber)
			if batches[key] == nil {
				batches[key] = &batch{}
			}
			batches[key].calls = append(batches[key].calls, struct {
				position int
				call     openAICompatibleToolCall
				event    model.SessionEvent
			}{position: int(position), call: call, event: event})
			callBatches[event.Ref.Id] = key
		}
	}
	for _, event := range events {
		switch event.Kind {
		case model.SessionEventKindMessageText:
			message, text, ok := transcriptMessageContent(event)
			if !ok {
				continue
			}
			if event.AuthorPrincipal != nil {
				messages = append(messages, openAICompatibleMessage{Role: "user", Content: text, Message: message})
			} else if event.AuthorAgent != nil {
				messages = append(messages, openAICompatibleMessage{Role: "assistant", Content: text, Message: message})
			}
		case model.SessionEventKindToolRequest:
			if event.AuthorAgent == nil {
				continue
			}
			call := calls[event.Ref.Id]
			if key, ok := callBatches[event.Ref.Id]; ok {
				current := batches[key]
				if current.emitted {
					continue
				}
				sort.Slice(current.calls, func(left, right int) bool {
					return current.calls[left].position < current.calls[right].position
				})
				batchCalls := make([]openAICompatibleToolCall, len(current.calls))
				events := make([]string, len(current.calls))
				for index, entry := range current.calls {
					batchCalls[index] = entry.call
					events[index] = transcriptToolCallEvent(entry.event, entry.call.ID)
				}
				messages = append(messages, openAICompatibleMessage{Role: "assistant", Content: transcriptEvents(events...)})
				messages = append(messages, openAICompatibleMessage{Role: "assistant", ToolCalls: batchCalls})
				current.emitted = true
				continue
			}
			messages = append(messages, openAICompatibleMessage{Role: "assistant", Content: transcriptEvents(transcriptToolCallEvent(event, call.ID))})
			messages = append(messages, openAICompatibleMessage{Role: "assistant", ToolCalls: []openAICompatibleToolCall{call}})
		case model.SessionEventKindToolSuccess, model.SessionEventKindToolFailure:
			if event.AuthorAgent == nil || event.Parent == nil {
				continue
			}
			call, ok := calls[event.Parent.Id]
			if !ok {
				return fmt.Errorf("session tool output %q has no preceding tool call", event.Ref.Id), nil
			}
			output, ok := event.Payload["output"].(string)
			if !ok {
				return fmt.Errorf("session tool output %q has no text output", event.Ref.Id), nil
			}
			status := "success"
			if event.Kind == model.SessionEventKindToolFailure {
				status = "failure"
			}
			messages = append(messages, openAICompatibleMessage{Role: "tool", ToolCallID: call.ID, Content: output, ToolOutput: &transcriptToolOutput{Event: event, Status: status}})
		}
	}
	return nil, messages
}

func openAICompatibleStoredToolCall(event model.SessionEvent) (openAICompatibleToolCall, error) {
	callID, callIDOK := event.Payload["call_id"].(string)
	name, nameOK := event.Payload["name"].(string)
	code, codeOK := event.Payload["code"].(string)
	reason, reasonOK := event.Payload["reason"].(string)
	if !callIDOK || callID == "" || !nameOK || name != "lisp" || !codeOK || strings.TrimSpace(code) == "" || !reasonOK || strings.TrimSpace(reason) == "" {
		return openAICompatibleToolCall{}, fmt.Errorf("session tool call %q is invalid", event.Ref.Id)
	}
	arguments, err := json.Marshal(struct {
		Code   string `json:"code"`
		Reason string `json:"reason"`
	}{Code: code, Reason: reason})
	if err != nil {
		return openAICompatibleToolCall{}, fmt.Errorf("encode session tool call %q arguments: %w", event.Ref.Id, err)
	}
	call := openAICompatibleToolCall{ID: callID, Type: "function"}
	call.Function.Name = name
	call.Function.Arguments = string(arguments)
	return call, nil
}

func toolOutputCreatedAt(callCreatedAt string) string {
	return eventTerminalCreatedAt(callCreatedAt)
}

func eventTerminalCreatedAt(startedAt string) string {
	started, err := time.Parse("2006-01-02T15:04:05.000Z", startedAt)
	if err != nil {
		return time.Now().UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	minimum := started.Add(time.Millisecond)
	if now.Before(minimum) {
		return minimum.Format("2006-01-02T15:04:05.000Z")
	}
	return now.Format("2006-01-02T15:04:05.000Z")
}

func openAIResponsesInput(messages []openAICompatibleMessage) []json.RawMessage {
	input := make([]json.RawMessage, 0, len(messages))
	for _, message := range messages {
		if message.Role == "assistant" && len(message.ToolCalls) > 0 {
			for _, call := range message.ToolCalls {
				input = append(input, openAIResponsesFunctionCall(call))
			}
			continue
		}
		if message.Role == "tool" {
			input = append(input, openAIResponsesFunctionOutput(message.ToolCallID, message.Content))
			continue
		}
		input = append(input, openAIResponsesMessage(message.Role, message.Content))
	}
	return input
}

func sessionEventReplyWorkflowID(event model.SessionEventRef) string {
	return "session-event-reply:" + event.Id
}

func sessionReplyCancellationWorkflowID(event model.SessionEventRef) string {
	return "session-reply-cancel:" + event.Id
}

func sessionNameWorkflowID(session model.SessionRef) string {
	return "session-name:" + sessionEventReplyPartition(session)
}

func sessionToolCallWorkflowID(request model.SessionEventRef) string {
	return "session-tool-call:" + request.Id
}

func sessionApprovalWorkflowID(request model.SessionEventRef) string {
	return "session-approval:" + request.Id
}

func sessionInputWorkflowID(request model.SessionEventRef) string {
	return "session-input:" + request.Id
}

func sessionEventReplyPartition(session model.SessionRef) string {
	return session.Workspace.Id + "/" + session.Id
}

func sessionEventReplyTaskPartition(event model.SessionEventRef) string {
	return sessionEventReplyPartition(event.Session) + "/" + event.Id
}
