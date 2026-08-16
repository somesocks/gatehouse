package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	"gatehouse/database"
	"gatehouse/diagnostics"
	"gatehouse/keychain"
	"gatehouse/lisp"
	"gatehouse/model"
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
)

type SessionEventReplyInput struct {
	Event model.SessionEventRef
}

type SessionNameInput struct {
	Session model.SessionRef
}

type SessionToolCallInput struct {
	Parent    model.SessionEventRef
	Agent     model.WorkspaceAgentRef
	Principal model.PrincipalRef
	Round     int
	Position  int
	CallID    string
	Code      string
	Reason    string
}

type sessionToolCallExecution struct {
	Kind   string
	Output string
}

type sessionReplyPreparation struct {
	Existing *model.SessionEvent
	Selected *database.WorkspaceAgentModel
	Message  *model.SessionEvent
	Messages []openAICompatibleMessage
}

type sessionNamePreparation struct {
	Existing *string
	Selected *database.WorkspaceAgentModel
	Text     string
}

type sessionReplyCancelled struct{}

func (sessionReplyCancelled) Error() string { return "reply cancelled" }

type sessionToolCallStoredOutput struct {
	Found  bool
	Output string
}

type SessionEventReplyRuntime struct {
	store *database.Store
	keyring *keychain.Keyring
	storage *storage.Client
	dbos  dbos.Context
	queue dbos.Queue
	toolQueue dbos.Queue
	nameQueue dbos.Queue
}

func NewSessionEventReplyRuntime(ctx dbos.Context, store *database.Store, keyring *keychain.Keyring) (error, *SessionEventReplyRuntime) {
	queue, err := dbos.RegisterQueue(ctx, sessionEventReplyQueue,
		dbos.WithPartitionQueue(),
		dbos.WithGlobalConcurrency(1),
	)
	if err != nil {
		return fmt.Errorf("register session event reply queue: %w", err), nil
	}
	toolQueue, err := dbos.RegisterQueue(ctx, sessionToolCallQueue, dbos.WithGlobalConcurrency(4))
	if err != nil {
		return fmt.Errorf("register session tool-call queue: %w", err), nil
	}
	nameQueue, err := dbos.RegisterQueue(ctx, sessionNameQueue, dbos.WithPartitionQueue(), dbos.WithGlobalConcurrency(1))
	if err != nil {
		return fmt.Errorf("register session name queue: %w", err), nil
	}
	runtime := &SessionEventReplyRuntime{store: store, keyring: keyring, storage: storage.NewClient(store, keyring), dbos: ctx, queue: queue, toolQueue: toolQueue, nameQueue: nameQueue}
	dbos.RegisterWorkflow(ctx, runtime.reply,
		dbos.WithInstance(runtime),
		dbos.WithWorkflowName("gatehouse.session-event-reply"),
	)
	dbos.RegisterWorkflow(ctx, runtime.toolCall,
		dbos.WithInstance(runtime),
		dbos.WithWorkflowName("gatehouse.session-tool-call"),
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

func (runtime *SessionEventReplyRuntime) Reconcile() error {
	err, tasks := runtime.store.SessionEventReplyTasksGet(runtime.dbos, 100)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		err, cancelled := runtime.replyCancellationComplete(runtime.dbos, task.Event)
		if err != nil {
			return err
		}
		if cancelled {
			if err := runtime.store.SessionEventReplyTaskDelete(runtime.dbos, task.Event); err != nil {
				return err
			}
			continue
		}
		_, err = dbos.RunWorkflow(runtime.dbos, runtime.reply, SessionEventReplyInput{Event: task.Event},
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
	return nil
}

func (runtime *SessionEventReplyRuntime) nameSession(ctx dbos.Context, input SessionNameInput) (string, error) {
	preparation, err := dbos.RunAsStep(ctx, func(step context.Context) (sessionNamePreparation, error) {
		err, existing := runtime.store.SessionNameGet(step, input.Session)
		if err != nil || existing != nil {
			return sessionNamePreparation{Existing: existing}, err
		}
		err, events := runtime.store.SessionEventsGet(step, input.Session)
		if err != nil {
			return sessionNamePreparation{}, err
		}
		for _, event := range events {
			if event.Parent != nil || event.Kind != "message.text" || event.AuthorPrincipal == nil {
				continue
			}
			text, ok := event.Payload["text"].(string)
			if !ok || strings.TrimSpace(text) == "" {
				continue
			}
			preferred, _ := event.Payload["agent"].(string)
			err, selected := runtime.store.WorkspaceAgentModelSelect(step, input.Session.Workspace, preferred)
			if err != nil {
				return sessionNamePreparation{}, err
			}
			if selected == nil {
				return sessionNamePreparation{}, fmt.Errorf("name session %q: no enabled workspace agent", input.Session.Id)
			}
			return sessionNamePreparation{Selected: selected, Text: text}, nil
		}
		return sessionNamePreparation{}, nil
	}, dbos.WithStepName("gatehouse.session-name-generate"))
	if err != nil {
		return "", err
	}
	if preparation.Existing != nil {
		return *preparation.Existing, nil
	}
	if preparation.Selected == nil {
		return "", nil
	}
	err, name := runtime.sessionNameCompletion(ctx, preparation.Selected, preparation.Text)
	if err != nil {
		return "", err
	}
	_, err = dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
		err, updated := runtime.store.SessionNameSet(step, input.Session, name)
		return updated, err
	}, dbos.WithStepName("gatehouse.session-name-persist"))
	if err != nil {
		return "", err
	}
	return name, nil
}

func (runtime *SessionEventReplyRuntime) sessionNameCompletion(ctx dbos.Context, selected *database.WorkspaceAgentModel, text string) (error, string) {
	switch selected.Protocol {
	case "builtin":
		return BuiltinReply(selected.Model, selected.Parameters)
	case "openai-chat-completions":
		if selected.BaseURL == nil || selected.Keychain == nil || selected.APIKey == nil {
			return fmt.Errorf("name session with provider %q: missing credentials", selected.ProviderID), ""
		}
		reply, err := runtime.openAICompatibleComplete(ctx, selected, openAICompatibleRequest{
			Model: selected.Model,
			Messages: []openAICompatibleMessage{{Role: "system", Content: sessionNamePrompt}, {Role: "user", Content: text}},
		})
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

func (runtime *SessionEventReplyRuntime) reply(ctx dbos.Context, input SessionEventReplyInput) (model.SessionEvent, error) {
	replyRef := model.SessionEventRef{Session: input.Event.Session, Id: sessionEventReplyID(input.Event)}
	preparation, err := dbos.RunAsStep(ctx, func(step context.Context) (sessionReplyPreparation, error) {
		err, existing := runtime.store.SessionEventGet(step, replyRef)
		if err != nil || existing != nil {
			return sessionReplyPreparation{Existing: existing}, err
		}
		err, message := runtime.store.SessionEventGet(step, input.Event)
		if err != nil {
			return sessionReplyPreparation{}, err
		}
		preferred := ""
		if message != nil && message.Kind == "message.text" && message.AuthorPrincipal != nil {
			preferred, _ = message.Payload["agent"].(string)
		}
		err, selected := runtime.store.WorkspaceAgentModelSelect(step, input.Event.Session.Workspace, preferred)
		if err != nil {
			return sessionReplyPreparation{}, err
		}
		err, events := runtime.store.SessionEventsGet(step, input.Event.Session)
		if err != nil {
			return sessionReplyPreparation{}, err
		}
		err, messages := openAICompatibleMessages(events)
		if err != nil {
			return sessionReplyPreparation{}, err
		}
		return sessionReplyPreparation{Selected: selected, Message: message, Messages: messages}, nil
	}, dbos.WithStepName("gatehouse.session-event-reply-prepare"))
	if err != nil {
		return model.SessionEvent{}, err
	}
	if preparation.Existing != nil {
		return *preparation.Existing, nil
	}
	if err := runtime.replyCancellationCheck(ctx, input.Event); err != nil {
		if _, cancelled := err.(sessionReplyCancelled); cancelled {
			return model.SessionEvent{}, nil
		}
		return model.SessionEvent{}, err
	}
	selected := preparation.Selected
	if selected == nil {
		return model.SessionEvent{}, fmt.Errorf("reply to session event %q: no enabled workspace agent", input.Event.Id)
	}
	message := preparation.Message
	if message == nil {
		return model.SessionEvent{}, fmt.Errorf("reply to session event %q: event not found", input.Event.Id)
	}
	messages := preparation.Messages
	if len(messages) == 0 {
		return model.SessionEvent{}, fmt.Errorf("reply to session event %q: message has no text", input.Event.Id)
	}
	var text string
	switch selected.Protocol {
	case "builtin":
		err, thinking := runtime.thinkingStart(ctx, input.Event, selected.Ref, 0)
		if err != nil {
			return model.SessionEvent{}, err
		}
		text, err = dbos.RunAsStep(ctx, func(context.Context) (string, error) {
			err, text := BuiltinReply(selected.Model, selected.Parameters)
			return text, err
		}, dbos.WithStepName("gatehouse.session-event-reply-builtin"))
		if err != nil {
			return model.SessionEvent{}, runtime.thinkingFinish(ctx, thinking, "thinking.failed", err)
		}
		if err := runtime.thinkingFinish(ctx, thinking, "thinking.completed", nil); err != nil {
			return model.SessionEvent{}, err
		}
	case "openai-chat-completions", "openai-responses":
		if message.AuthorPrincipal == nil {
			return model.SessionEvent{}, fmt.Errorf("reply to session event %q: Lisp authorization requires a principal author", input.Event.Id)
		}
		err, text = runtime.openAIReply(ctx, input.Event, selected, messages, message.AuthorPrincipal.Ref)
	default:
		err = fmt.Errorf("unsupported provider protocol %q", selected.Protocol)
	}
	if err != nil {
		if _, cancelled := err.(sessionReplyCancelled); cancelled {
			return model.SessionEvent{}, nil
		}
		return model.SessionEvent{}, err
	}
	if err := runtime.replyCancellationCheck(ctx, input.Event); err != nil {
		if _, cancelled := err.(sessionReplyCancelled); cancelled {
			return model.SessionEvent{}, nil
		}
		return model.SessionEvent{}, err
	}
	event := model.SessionEvent{
		Ref:         replyRef,
		Parent:      &input.Event,
		Kind:        "message.text",
		AuthorAgent: &selected.Ref,
		Payload:     map[string]interface{}{"text": text},
	}
	stored, err := dbos.RunAsStep(ctx, func(step context.Context) (model.SessionEvent, error) {
		err, stored := runtime.sessionEventGetOrCreate(step, event)
		return stored, err
	}, dbos.WithStepName("gatehouse.session-event-reply-persist"))
	if err != nil {
		return model.SessionEvent{}, err
	}
	return stored, nil
}

func (runtime *SessionEventReplyRuntime) openAIReply(ctx dbos.Context, parent model.SessionEventRef, selected *database.WorkspaceAgentModel, messages []openAICompatibleMessage, principal model.PrincipalRef) (error, string) {
	if selected.BaseURL == nil || selected.Keychain == nil || selected.APIKey == nil {
		return fmt.Errorf("reply with OpenAI-compatible provider %q: missing credentials", selected.ProviderID), ""
	}
	err, reasoningEffort := openAICompatibleReasoningEffort(selected.Parameters)
	if err != nil {
		return err, ""
	}
	if selected.Protocol == "openai-responses" {
		return runtime.openAIResponsesReply(ctx, parent, selected, messages, principal, reasoningEffort)
	}
	requestMessages := openAIRequestMessages(selected, messages)
	callCount := 0
	for round := 0; ; round++ {
		if err := runtime.replyCancellationCheck(ctx, parent); err != nil {
			return err, ""
		}
		err, thinking := runtime.thinkingStart(ctx, parent, selected.Ref, round)
		if err != nil {
			return err, ""
		}
		reply, err := runtime.openAICompatibleComplete(ctx, selected, openAICompatibleRequest{
				Model: selected.Model, Messages: requestMessages, Tools: []openAICompatibleTool{openAICompatibleLispTool()}, ParallelToolCalls: true, ReasoningEffort: reasoningEffort,
			})
		if err != nil {
			return runtime.thinkingFinish(ctx, thinking, "thinking.failed", err), ""
		}
		if len(reply.ToolCalls) == 0 {
			if strings.TrimSpace(reply.Content) == "" {
				err := fmt.Errorf("OpenAI-compatible completion returned no message")
				return runtime.thinkingFinish(ctx, thinking, "thinking.failed", err), ""
			}
			if err := runtime.thinkingFinish(ctx, thinking, "thinking.completed", nil); err != nil {
				return err, ""
			}
			return nil, reply.Content
		}
		if round >= selected.MaxTurns {
			err := fmt.Errorf("OpenAI-compatible completion exceeded turn limit")
			return runtime.thinkingFinish(ctx, thinking, "thinking.failed", err), ""
		}
		if err := runtime.thinkingFinish(ctx, thinking, "thinking.completed", nil); err != nil {
			return err, ""
		}
		if err := runtime.replyCancellationCheck(ctx, parent); err != nil {
			return err, ""
		}
		output, err := runtime.runLispCalls(ctx, parent, selected.Ref, principal, round, callCount, reply.ToolCalls)
		if err != nil {
			return err, ""
		}
		callCount += len(reply.ToolCalls)
		requestMessages = append(requestMessages, reply)
		for index, call := range reply.ToolCalls {
			requestMessages = append(requestMessages, openAICompatibleMessage{Role: "tool", ToolCallID: call.ID, Content: output[index]})
		}
	}
}

func (runtime *SessionEventReplyRuntime) openAICompatibleComplete(ctx dbos.Context, selected *database.WorkspaceAgentModel, request openAICompatibleRequest) (openAICompatibleMessage, error) {
	return dbos.RunAsStep(ctx, func(step context.Context) (openAICompatibleMessage, error) {
		err, apiKey := runtime.openAIAPIKey(step, selected)
		if err != nil {
			return openAICompatibleMessage{}, err
		}
		defer clear(apiKey)
		err, reply := OpenAICompatibleComplete(step, &http.Client{Timeout: time.Minute}, *selected.BaseURL, string(apiKey), request)
		return reply, err
	}, dbos.WithStepName("gatehouse.session-event-reply-completion"))
}

func (runtime *SessionEventReplyRuntime) openAIResponsesComplete(ctx dbos.Context, selected *database.WorkspaceAgentModel, request openAIResponsesRequest) (openAIResponsesResponse, error) {
	return dbos.RunAsStep(ctx, func(step context.Context) (openAIResponsesResponse, error) {
		err, apiKey := runtime.openAIAPIKey(step, selected)
		if err != nil {
			return openAIResponsesResponse{}, err
		}
		defer clear(apiKey)
		err, reply := OpenAIResponsesComplete(step, &http.Client{Timeout: time.Minute}, *selected.BaseURL, string(apiKey), request)
		return reply, err
	}, dbos.WithStepName("gatehouse.session-event-reply-responses"))
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
	decryptedErr, apiKey := keychain.Open(key, []byte("gh=v1|agent-provider="+selected.ProviderID), encrypted)
	clear(key)
	clear(keys)
	if decryptedErr != nil {
		return fmt.Errorf("decrypt API key for provider %q: %w", selected.ProviderID, decryptedErr), nil
	}
	return nil, apiKey
}

func (runtime *SessionEventReplyRuntime) toolCall(ctx dbos.Context, input SessionToolCallInput) (string, error) {
	storedCall, err := dbos.RunAsStep(ctx, func(step context.Context) (model.SessionEvent, error) {
	callEvent := model.SessionEvent{
			Ref: model.SessionEventRef{Session: input.Parent.Session, Id: sessionEventReplyChildID(input.Parent, "tool.request", input.Position)}, Parent: &input.Parent, Kind: "tool.request", AuthorAgent: &input.Agent,
			Payload: map[string]interface{}{"name": "lisp", "call_id": input.CallID, "code": input.Code, "reason": input.Reason, "batch": input.Round, "position": input.Position},
		}
		err, event := runtime.sessionEventGetOrCreate(step, callEvent)
		if err != nil {
			return model.SessionEvent{}, err
		}
		return event, nil
	}, dbos.WithStepName("gatehouse.session-tool-call-persist"))
	if err != nil {
		return "", err
	}
	storedOutput, err := dbos.RunAsStep(ctx, func(step context.Context) (sessionToolCallStoredOutput, error) {
		for _, kind := range []string{"tool.success", "tool.failure"} {
			resultRef := model.SessionEventRef{Session: input.Parent.Session, Id: sessionEventReplyChildID(input.Parent, kind, input.Position)}
			err, existing := runtime.store.SessionEventGet(step, resultRef)
			if err != nil {
				return sessionToolCallStoredOutput{}, err
			}
			if existing != nil {
				output, _ := existing.Payload["output"].(string)
				return sessionToolCallStoredOutput{Found: true, Output: output}, nil
			}
		}
		return sessionToolCallStoredOutput{}, nil
	}, dbos.WithStepName("gatehouse.session-tool-call-load-output"))
	if err != nil {
		return "", err
	}
	if storedOutput.Found {
		return storedOutput.Output, nil
	}
	execution, err := dbos.RunAsStep(ctx, func(step context.Context) (sessionToolCallExecution, error) {
		err, tools, resources, files, values := runtime.turnEnvironment(step, input.Parent.Session, input.Principal)
		if err != nil {
			return sessionToolCallExecution{Kind: "tool.failure", Output: err.Error()}, nil
		}
		defer clearResourceValues(values)
		call, diagnosticErr := diagnostics.Begin("agent.tool_call.evaluate", "")
		if diagnosticErr != nil {
			return sessionToolCallExecution{Kind: "tool.failure", Output: diagnosticErr.Error()}, nil
		}
		evalErr, result := lisp.RunTurnWithFiles(input.Code, tools, resources, files, func(id string, offset, length int64) (error, []byte) {
			err, file, _ := runtime.store.SessionFileGet(step, model.SessionFileRef{Session: input.Parent.Session, Id: id}, input.Principal)
			if err != nil || file == nil {
				if err != nil {
					return err, nil
				}
				return fmt.Errorf("read session file: unavailable"), nil
			}
			return runtime.storage.Read(step, file.StorageObject.Id, offset, length)
		})
		evalErr = call.End(evalErr)
		if evalErr != nil {
			return sessionToolCallExecution{Kind: "tool.failure", Output: evalErr.Error()}, nil
		}
		return sessionToolCallExecution{Kind: "tool.success", Output: result.String()}, nil
	}, dbos.WithStepName("gatehouse.session-tool-call-evaluate"))
	if err != nil {
		return "", err
	}
	_, err = dbos.RunAsStep(ctx, func(step context.Context) (model.SessionEvent, error) {
		resultEvent := model.SessionEvent{
			Ref: model.SessionEventRef{Session: input.Parent.Session, Id: sessionEventReplyChildID(input.Parent, execution.Kind, input.Position)}, Parent: &storedCall.Ref, Kind: execution.Kind, AuthorAgent: &input.Agent,
			Payload: map[string]interface{}{"name": "lisp", "call_id": input.CallID, "output": execution.Output}, CreatedAt: toolOutputCreatedAt(storedCall.CreatedAt),
		}
		err, event := runtime.sessionEventGetOrCreate(step, resultEvent)
		return event, err
	}, dbos.WithStepName("gatehouse.session-tool-call-persist-output"))
	if err != nil {
		return "", err
	}
	return execution.Output, nil
}

func (runtime *SessionEventReplyRuntime) runLispCalls(ctx dbos.Context, parent model.SessionEventRef, agent model.WorkspaceAgentRef, principal model.PrincipalRef, round, offset int, calls []openAICompatibleToolCall) ([]string, error) {
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
		inputs[index] = SessionToolCallInput{Parent: parent, Agent: agent, Principal: principal, Round: round, Position: offset + index, CallID: call.ID, Code: code, Reason: reason}
	}
	handles := make([]dbos.WorkflowHandle[string], len(inputs))
	for index, input := range inputs {
		handle, err := dbos.RunWorkflow(ctx, runtime.toolCall, input,
			dbos.WithRunInstance(runtime),
			dbos.WithWorkflowID(sessionToolCallWorkflowID(parent, input.Position)),
			dbos.WithQueue(runtime.toolQueue),
		)
		if err != nil {
			return nil, fmt.Errorf("start session tool call %q: %w", input.CallID, err)
		}
		handles[index] = handle
	}
	outputs := make([]string, len(handles))
	for index, handle := range handles {
		output, err := handle.GetResult()
		if err != nil {
			return nil, fmt.Errorf("await session tool call %q: %w", inputs[index].CallID, err)
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

const openAISystemPrompt = `# Role

You are an agent that completes user requests using authorized workspace capabilities. Use available tools and resources to perform the work, not merely describe how it could be done. Verify results before reporting completion, and continue working when verification shows the result is missing, invalid, or incomplete. Communicate clear user-facing results without mentioning your tools or implementation details.

# Tools

You have one tool, ` + "`lisp`" + `. The Lisp environment contains all authorized workspace capabilities and resources. It is a custom Lisp dialect, not Common Lisp or Scheme. Use its discovery bindings to learn available capabilities.

Session file attachments are available through ` + "`file/list`" + ` and ` + "`file/read`" + `. Inspect file metadata first, then read only the ranges needed to complete the request.

Examples:

- ` + "`(help/env) ; List all bindings.`" + `
- ` + "`(help/env \"prefix\") ; List bindings matching a prefix.`" + `
- ` + "`(help/search \"term\") ; Search documented capabilities.`" + `
- ` + "`(help 'binding) ; Inspect a binding.`" + `
- ` + "`(let ((factor 2) (value 21)) (* factor value)) ; Uses lexical bindings and returns 42.`" + `
- ` + "`(list/map (fn (number) (* number number)) (list 1 2 3)) ; Returns (1 4 9).`" + `
- ` + "`(apply + (list 1 2 3)) ; Calls a function with values from a list.`" + `

# Instructions

Treat tool failures as feedback. Correct and retry when the request remains answerable. Return unexecuted code only when the user explicitly asks for code rather than its result.`

func openAISystemPromptFor(selected *database.WorkspaceAgentModel) string {
	if selected.SystemPrompt != nil {
		return *selected.SystemPrompt
	}
	return openAISystemPrompt
}

func openAIRequestMessages(selected *database.WorkspaceAgentModel, messages []openAICompatibleMessage) []openAICompatibleMessage {
	prompt := openAISystemPromptFor(selected)
	if prompt == "" {
		return messages
	}
	return append([]openAICompatibleMessage{{Role: "system", Content: prompt}}, messages...)
}

func (runtime *SessionEventReplyRuntime) openAIResponsesReply(ctx dbos.Context, parent model.SessionEventRef, selected *database.WorkspaceAgentModel, messages []openAICompatibleMessage, principal model.PrincipalRef, reasoningEffort string) (error, string) {
	input := openAIResponsesInput(messages)
	var reasoning *openAIResponsesReasoning
	if reasoningEffort != "" {
		reasoning = &openAIResponsesReasoning{Effort: reasoningEffort}
	}
	callCount := 0
	for round := 0; ; round++ {
		if err := runtime.replyCancellationCheck(ctx, parent); err != nil {
			return err, ""
		}
		err, thinking := runtime.thinkingStart(ctx, parent, selected.Ref, round)
		if err != nil {
			return err, ""
		}
		reply, err := runtime.openAIResponsesComplete(ctx, selected, openAIResponsesRequest{
			Model: selected.Model, Instructions: openAISystemPromptFor(selected), Input: input, Tools: []openAIResponsesTool{openAIResponsesLispTool()}, ParallelToolCalls: true, Reasoning: reasoning,
			})
		if err != nil {
			return runtime.thinkingFinish(ctx, thinking, "thinking.failed", err), ""
		}
		input = append(input, reply.Output...)
		calls := make([]openAICompatibleToolCall, 0, 1)
		text := ""
		for _, raw := range reply.Output {
			var output openAIResponsesOutput
			if err := json.Unmarshal(raw, &output); err != nil {
				err := fmt.Errorf("decode OpenAI Responses output: %w", err)
				return runtime.thinkingFinish(ctx, thinking, "thinking.failed", err), ""
			}
			switch output.Type {
			case "function_call":
				call := openAICompatibleToolCall{ID: output.CallID, Type: "function"}
				call.Function.Name = output.Name
				call.Function.Arguments = output.Arguments
				calls = append(calls, call)
			case "message":
				for _, content := range output.Content {
					if content.Type == "output_text" {
						text += content.Text
					}
				}
			}
		}
		if len(calls) == 0 {
			if strings.TrimSpace(text) == "" {
				err := fmt.Errorf("OpenAI Responses returned no message")
				return runtime.thinkingFinish(ctx, thinking, "thinking.failed", err), ""
			}
			if err := runtime.thinkingFinish(ctx, thinking, "thinking.completed", nil); err != nil {
				return err, ""
			}
			return nil, text
		}
		if round >= selected.MaxTurns {
			err := fmt.Errorf("OpenAI Responses exceeded turn limit")
			return runtime.thinkingFinish(ctx, thinking, "thinking.failed", err), ""
		}
		if err := runtime.thinkingFinish(ctx, thinking, "thinking.completed", nil); err != nil {
			return err, ""
		}
		if err := runtime.replyCancellationCheck(ctx, parent); err != nil {
			return err, ""
		}
		outputs, err := runtime.runLispCalls(ctx, parent, selected.Ref, principal, round, callCount, calls)
		if err != nil {
			return err, ""
		}
		callCount += len(calls)
		for index, call := range calls {
			input = append(input, openAIResponsesFunctionOutput(call.ID, outputs[index]))
		}
	}
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

func (runtime *SessionEventReplyRuntime) sessionEventGetOrCreate(ctx context.Context, event model.SessionEvent) (error, model.SessionEvent) {
	err, existing := runtime.store.SessionEventGet(ctx, event.Ref)
	if err != nil {
		return err, model.SessionEvent{}
	}
	if existing != nil {
		return nil, *existing
	}
	return runtime.store.SessionEventsCreate(ctx, event)
}

func (runtime *SessionEventReplyRuntime) replyCancellationRequested(ctx context.Context, parent model.SessionEventRef) (error, *model.SessionEvent) {
	return runtime.store.SessionEventChildGet(ctx, parent, "cancel.request")
}

func (runtime *SessionEventReplyRuntime) replyCancellationCheck(ctx dbos.Context, parent model.SessionEventRef) error {
	cancelled, err := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
		err, cancelled := runtime.replyCancellationComplete(step, parent)
		return cancelled, err
	}, dbos.WithStepName("gatehouse.session-event-reply-cancellation-check"))
	if err != nil {
		return err
	}
	if !cancelled {
		return nil
	}
	return sessionReplyCancelled{}
}

func (runtime *SessionEventReplyRuntime) replyCancellationComplete(ctx context.Context, parent model.SessionEventRef) (error, bool) {
	err, request := runtime.replyCancellationRequested(ctx, parent)
	if err != nil {
		return err, false
	}
	if request == nil {
		return nil, false
	}
	event := model.SessionEvent{
		Ref: model.SessionEventRef{Session: parent.Session, Id: sessionEventReplyChildID(parent, "cancel.success", 0)}, Parent: &request.Ref, Kind: "cancel.success", AuthorPrincipal: request.AuthorPrincipal,
		Payload: map[string]interface{}{}, CreatedAt: eventTerminalCreatedAt(request.CreatedAt),
	}
	err, _ = runtime.sessionEventGetOrCreate(ctx, event)
	return err, err == nil
}

func (runtime *SessionEventReplyRuntime) thinkingStart(ctx dbos.Context, parent model.SessionEventRef, agent model.WorkspaceAgentRef, turn int) (error, model.SessionEvent) {
	started, err := dbos.RunAsStep(ctx, func(step context.Context) (model.SessionEvent, error) {
		event := model.SessionEvent{
			Ref: model.SessionEventRef{Session: parent.Session, Id: sessionEventReplyChildID(parent, "thinking.started", turn)}, Parent: &parent, Kind: "thinking.started", AuthorAgent: &agent,
			Payload: map[string]interface{}{"turn": turn},
		}
		err, stored := runtime.sessionEventGetOrCreate(step, event)
		return stored, err
	}, dbos.WithStepName("gatehouse.session-event-thinking-start"))
	return err, started
}

func (runtime *SessionEventReplyRuntime) thinkingFinish(ctx dbos.Context, started model.SessionEvent, kind string, completionErr error) error {
	_, err := dbos.RunAsStep(ctx, func(step context.Context) (model.SessionEvent, error) {
		event := model.SessionEvent{
			Ref: model.SessionEventRef{Session: started.Ref.Session, Id: sessionEventReplyChildID(started.Ref, kind, 0)}, Parent: &started.Ref, Kind: kind, AuthorAgent: started.AuthorAgent,
			Payload: map[string]interface{}{}, CreatedAt: eventTerminalCreatedAt(started.CreatedAt),
		}
		err, stored := runtime.sessionEventGetOrCreate(step, event)
		return stored, err
	}, dbos.WithStepName("gatehouse.session-event-thinking-finish"))
	if completionErr != nil && err != nil {
		return errors.Join(completionErr, err)
	}
	if completionErr != nil {
		return completionErr
	}
	return err
}

func openAICompatibleLispTool() openAICompatibleTool {
	tool := openAICompatibleTool{Type: "function"}
	tool.Function.Name = "lisp"
	tool.Function.Description = "Evaluate Lisp using only the workspace's authorized tools and resources. Explain the purpose in reason. Use (help/env) and (help/search) to discover this Lisp dialect; it is not Common Lisp. Correct and retry failed evaluations when possible."
	tool.Function.Parameters = map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"code", "reason"},
		"properties": map[string]any{
			"code": map[string]any{"type": "string", "description": "Lisp expression to evaluate."},
			"reason": map[string]any{"type": "string", "description": "Why this evaluation is needed."},
		},
	}
	return tool
}

func openAIResponsesLispTool() openAIResponsesTool {
	compatible := openAICompatibleLispTool()
	return openAIResponsesTool{Type: "function", Name: compatible.Function.Name, Description: compatible.Function.Description, Parameters: compatible.Function.Parameters, Strict: true}
}

func readToolSource(source string) (error, string) {
	if !strings.HasPrefix(source, "file:") {
		return fmt.Errorf("unsupported tool source %q", source), ""
	}
	path := strings.TrimPrefix(source, "file:")
	if path == "" {
		return fmt.Errorf("file source path is empty"), ""
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return err, ""
	}
	return nil, string(contents)
}

func clearResourceValues(values map[model.ResourceRef][]byte) {
	for _, value := range values {
		clear(value)
	}
}

func (runtime *SessionEventReplyRuntime) turnEnvironment(ctx context.Context, session model.SessionRef, principal model.PrincipalRef) (error, []lisp.TurnTool, []lisp.TurnResource, []lisp.TurnFile, map[model.ResourceRef][]byte) {
	workspace := session.Workspace
	err, configuredTools := runtime.store.WorkspaceToolsGet(ctx, workspace, principal)
	if err != nil {
		return err, nil, nil, nil, nil
	}
	err, configuredResources := runtime.store.WorkspaceResourcesGet(ctx, workspace, principal)
	if err != nil {
		return err, nil, nil, nil, nil
	}
	err, values := runtime.resolveResources(ctx, configuredResources)
	if err != nil {
		return err, nil, nil, nil, nil
	}
	tools := make([]lisp.TurnTool, 0, len(configuredTools))
	for _, configured := range configuredTools {
		err, source := readToolSource(configured.Source)
		if err != nil {
			clearResourceValues(values)
			return fmt.Errorf("read tool %q: %w", configured.Ref.Id, err), nil, nil, nil, nil
		}
		tools = append(tools, lisp.TurnTool{ID: configured.Ref.Id, Source: source})
	}
	turnResources := make([]lisp.TurnResource, 0, len(configuredResources))
	for _, configured := range configuredResources {
		turnResources = append(turnResources, lisp.TurnResource{ID: configured.Ref.Id, Value: values[configured.Ref], Secret: configured.Secret})
	}
	err, summaries := runtime.store.SessionFilesGet(ctx, session)
	if err != nil {
		clearResourceValues(values)
		return err, nil, nil, nil, nil
	}
	files := make([]lisp.TurnFile, 0, len(summaries))
	for _, file := range summaries {
		files = append(files, lisp.TurnFile{ID: file.ID, Name: file.Name, MediaType: file.MediaType, Size: file.Size, Fingerprint: file.Fingerprint})
	}
	return nil, tools, turnResources, files, values
}

func (runtime *SessionEventReplyRuntime) resolveResources(ctx context.Context, resources []model.Resource) (error, map[model.ResourceRef][]byte) {
	values := make(map[model.ResourceRef][]byte, len(resources))
	type encryptedResource struct {
		resource  model.Resource
		encrypted keychain.Encrypted
	}
	encryptedResources := make([]encryptedResource, 0)
	references := make([]model.KeychainRef, 0)
	for _, configured := range resources {
		ref := configured.Ref
		switch {
		case strings.HasPrefix(configured.Source, "file:"):
			path := strings.TrimPrefix(configured.Source, "file:")
			if path == "" {
				return fmt.Errorf("resolve resource %q: file source path is empty", configured.Ref.Id), nil
			}
			value, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("resolve resource %q from file %q: %w", configured.Ref.Id, path, err), nil
			}
			values[ref] = value
		case strings.HasPrefix(configured.Source, "env:"):
			name := strings.TrimPrefix(configured.Source, "env:")
			if name == "" {
				return fmt.Errorf("resolve resource %q: environment source name is empty", configured.Ref.Id), nil
			}
			value, ok := os.LookupEnv(name)
			if !ok {
				return fmt.Errorf("resolve resource %q: environment variable %q is not set", configured.Ref.Id, name), nil
			}
			values[ref] = []byte(value)
		case strings.HasPrefix(configured.Source, "gh-enc:"):
			err, encrypted := keychain.ParseResource(configured.Source)
			if err != nil {
				return fmt.Errorf("resolve resource %q: %w", configured.Ref.Id, err), nil
			}
			encryptedResources = append(encryptedResources, encryptedResource{resource: configured, encrypted: encrypted})
			references = append(references, *encrypted.Key)
		default:
			return fmt.Errorf("resolve resource %q: unsupported source %q", configured.Ref.Id, configured.Source), nil
		}
	}
	if len(references) == 0 {
		return nil, values
	}
	err, keys := runtime.keyring.Get(ctx, references)
	if err != nil {
		return err, nil
	}
	defer clear(keys)
	for _, configured := range encryptedResources {
		key, ok := keys[*configured.encrypted.Key]
		if !ok {
			return fmt.Errorf("resolve resource %q: keychain key %q version %d is unavailable", configured.resource.Ref.Id, configured.encrypted.Key.Id, configured.encrypted.Key.Version), nil
		}
		valueErr, value := keychain.Open(key, []byte("gh=v1|workspace="+configured.resource.Ref.Workspace.Id+"|resource="+configured.resource.Ref.Id), configured.encrypted)
		if valueErr != nil {
			return fmt.Errorf("resolve resource %q: %w", configured.resource.Ref.Id, valueErr), nil
		}
		values[configured.resource.Ref] = value
	}
	return nil, values
}

func openAICompatibleMessages(events []model.SessionEvent) (error, []openAICompatibleMessage) {
	messages := make([]openAICompatibleMessage, 0, len(events))
	calls := map[string]openAICompatibleToolCall{}
	type batch struct {
		calls []struct {
			position int
			call     openAICompatibleToolCall
		}
		emitted bool
	}
	batches := map[string]*batch{}
	callBatches := map[string]string{}
	for _, event := range events {
		if event.Kind != "tool.request" || event.AuthorAgent == nil {
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
			}{position: int(position), call: call})
			callBatches[event.Ref.Id] = key
		}
	}
	for _, event := range events {
		switch event.Kind {
		case "message.text":
			text, ok := event.Payload["text"].(string)
			attachments := openAICompatibleMessageAttachments(event.Payload["files"])
			if (!ok || strings.TrimSpace(text) == "") && attachments == "" {
				continue
			}
			if attachments != "" {
				if strings.TrimSpace(text) != "" {
					text += "\n\n"
				}
				text += attachments
			}
			if event.AuthorPrincipal != nil {
				messages = append(messages, openAICompatibleMessage{Role: "user", Content: text})
			} else if event.AuthorAgent != nil {
				messages = append(messages, openAICompatibleMessage{Role: "assistant", Content: text})
			}
		case "tool.request":
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
				for index, entry := range current.calls {
					batchCalls[index] = entry.call
				}
				messages = append(messages, openAICompatibleMessage{Role: "assistant", ToolCalls: batchCalls})
				current.emitted = true
				continue
			}
			messages = append(messages, openAICompatibleMessage{Role: "assistant", ToolCalls: []openAICompatibleToolCall{call}})
		case "tool.success", "tool.failure":
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
			messages = append(messages, openAICompatibleMessage{Role: "tool", ToolCallID: call.ID, Content: output})
		}
	}
	return nil, messages
}

func openAICompatibleMessageAttachments(value any) string {
	files, ok := value.([]interface{})
	if !ok || len(files) == 0 {
		return ""
	}
	lines := make([]string, 0, len(files)+1)
	lines = append(lines, "Attached session files:")
	for _, value := range files {
		file, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		id, idOK := file["id"].(string)
		name, nameOK := file["name"].(string)
		size, sizeOK := file["size"].(float64)
		fingerprint, fingerprintOK := file["fingerprint"].(string)
		if !idOK || !nameOK || !sizeOK || !fingerprintOK {
			continue
		}
		line := fmt.Sprintf("- %s (id: %s, size: %.0f bytes, fingerprint: %s", name, id, size, fingerprint)
		if mediaType, ok := file["media_type"].(string); ok && mediaType != "" {
			line += ", media type: " + mediaType
		}
		lines = append(lines, line+")")
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
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

func sessionNameWorkflowID(session model.SessionRef) string {
	return "session-name:" + sessionEventReplyPartition(session)
}

func sessionToolCallWorkflowID(event model.SessionEventRef, position int) string {
	return "session-tool-call:" + sessionEventReplyChildID(event, "workflow", position)
}

func sessionEventReplyPartition(session model.SessionRef) string {
	return session.Workspace.Id + "/" + session.Id
}

func sessionEventReplyID(event model.SessionEventRef) string {
	return sessionEventID(event, "reply")
}

func sessionEventReplyChildID(event model.SessionEventRef, kind string, index int) string {
	return sessionEventID(event, kind+"\x00"+fmt.Sprintf("%d", index))
}

func sessionEventID(event model.SessionEventRef, suffix string) string {
	id, err := typed_id.Derive(typed_id.SessionEvent, event.Session.Workspace.Id+"\x00"+event.Session.Id+"\x00"+event.Id+"\x00"+suffix)
	if err != nil {
		panic(err)
	}
	return id
}
