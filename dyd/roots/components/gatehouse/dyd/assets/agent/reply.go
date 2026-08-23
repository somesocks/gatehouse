package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	sessionApprovalWait    = time.Duration(1<<63 - 1)
)

type SessionEventReplyInput struct {
	Event model.SessionEventRef
}

type SessionNameInput struct {
	Session model.SessionRef
}

type SessionToolCallInput struct {
	Request   model.SessionEvent
	Agent     model.WorkspaceAgentRef
	Principal model.PrincipalRef
	CallID    string
	Code      string
	Reason    string
}

type SessionApprovalInput struct {
	Request model.SessionEventRef
}

type ApprovalOutcome string

const (
	ApprovalGranted  ApprovalOutcome = "approved"
	ApprovalRejected ApprovalOutcome = "rejected"
)

type sessionToolCallExecution struct {
	Kind   string
	Output string
}

type sessionReplyPreparation struct {
	Selected *database.WorkspaceAgentModel
	Message  *model.SessionEvent
}

type agentFinalReply struct {
	Text        string
	Attachments []string
}

type sessionNamePreparation struct {
	Existing *string
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
	toolQueue, err := dbos.RegisterQueue(ctx, sessionToolCallQueue)
	if err != nil {
		return fmt.Errorf("register session tool-call queue: %w", err), nil
	}
	nameQueue, err := dbos.RegisterQueue(ctx, sessionNameQueue, dbos.WithPartitionQueue(), dbos.WithGlobalConcurrency(1))
	if err != nil {
		return fmt.Errorf("register session name queue: %w", err), nil
	}
	dataSource, err := dbos.NewDataSource(ctx, store.DB, dbos.WithDataSourceName("gatehouse"))
	if err != nil {
		return fmt.Errorf("register Gatehouse database data source: %w", err), nil
	}
	runtime := &SessionEventReplyRuntime{store: store, keyring: keyring, storage: storage.NewClient(store, keyring), dbos: ctx, dataSource: dataSource, queue: queue, toolQueue: toolQueue, nameQueue: nameQueue}
	dbos.RegisterWorkflow(ctx, runtime.reply,
		dbos.WithInstance(runtime),
		dbos.WithWorkflowName("gatehouse.session-event-reply"),
	)
	dbos.RegisterWorkflow(ctx, runtime.toolCall,
		dbos.WithInstance(runtime),
		dbos.WithWorkflowName("gatehouse.session-tool-call"),
	)
	dbos.RegisterWorkflow(ctx, runtime.approval,
		dbos.WithInstance(runtime),
		dbos.WithWorkflowName("gatehouse.session-approval"),
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
			Model:    selected.Model,
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
	preparation, err := dbos.RunAsStep(ctx, func(step context.Context) (sessionReplyPreparation, error) {
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
		return sessionReplyPreparation{Selected: selected, Message: message}, nil
	}, dbos.WithStepName("gatehouse.session-event-reply-prepare"))
	if err != nil {
		return model.SessionEvent{}, err
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
	final := agentFinalReply{}
	switch selected.Protocol {
	case "builtin":
		err, thinking := runtime.thinkingStart(ctx, input.Event, selected.Ref, 0)
		if err != nil {
			return model.SessionEvent{}, err
		}
		final.Text, err = dbos.RunAsStep(ctx, func(context.Context) (string, error) {
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
		err, final = runtime.openAIReply(ctx, input.Event, selected, message.AuthorPrincipal.Ref)
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
	if len(final.Attachments) > 0 {
		final.Attachments, err = dbos.RunAsStep(ctx, func(step context.Context) ([]string, error) {
			err, references := runtime.store.SessionFileReferencesFilter(step, input.Event.Session, final.Attachments)
			if err != nil {
				return nil, err
			}
			attachments := make([]string, len(references))
			for index, reference := range references {
				attachments[index] = reference.ID
			}
			return attachments, nil
		}, dbos.WithStepName("gatehouse.session-event-reply-attachments"))
		if err != nil {
			return model.SessionEvent{}, fmt.Errorf("filter agent reply attachments: %w", err)
		}
	}
	event := model.SessionEvent{
		Ref:         model.SessionEventRef{Session: input.Event.Session},
		Parent:      &input.Event,
		Kind:        "message.text",
		AuthorAgent: &selected.Ref,
		Payload:     map[string]interface{}{"text": final.Text},
	}
	if len(final.Attachments) > 0 {
		event.Payload["attachments"] = final.Attachments
	}
	err, stored := runtime.persistAgentEvent(ctx, event)
	if err != nil {
		return model.SessionEvent{}, err
	}
	return stored, nil
}

func (runtime *SessionEventReplyRuntime) openAIReply(ctx dbos.Context, parent model.SessionEventRef, selected *database.WorkspaceAgentModel, principal model.PrincipalRef) (error, agentFinalReply) {
	if selected.BaseURL == nil || selected.Keychain == nil || selected.APIKey == nil {
		return fmt.Errorf("reply with OpenAI-compatible provider %q: missing credentials", selected.ProviderID), agentFinalReply{}
	}
	err, reasoningEffort := openAICompatibleReasoningEffort(selected.Parameters)
	if err != nil {
		return err, agentFinalReply{}
	}
	if selected.Protocol == "openai-responses" {
		return runtime.openAIResponsesReply(ctx, parent, selected, principal, reasoningEffort)
	}
	callCount := 0
	for round := 0; ; round++ {
		if err := runtime.replyCancellationCheck(ctx, parent); err != nil {
			return err, agentFinalReply{}
		}
		err, messages := runtime.agentContext(ctx, parent, selected)
		if err != nil {
			return err, agentFinalReply{}
		}
		requestMessages := openAIRequestMessages(selected, messages)
		err, thinking := runtime.thinkingStart(ctx, parent, selected.Ref, round)
		if err != nil {
			return err, agentFinalReply{}
		}
		reply, err := runtime.openAICompatibleComplete(ctx, selected, openAICompatibleRequest{
			Model: selected.Model, Messages: requestMessages, Tools: []openAICompatibleTool{openAICompatibleLispTool()}, ResponseFormat: openAIFinalReplyResponseFormat(), ParallelToolCalls: true, ReasoningEffort: reasoningEffort, MaxTokens: selected.MaxOutputTokens,
		})
		if err != nil {
			return runtime.thinkingFinish(ctx, thinking, "thinking.failed", err), agentFinalReply{}
		}
		if len(reply.ToolCalls) == 0 {
			final, err := openAIFinalReply(reply.Content)
			if err != nil {
				logInvalidOpenAIFinalReply(selected.ProviderID, parent, reply.Content)
				return runtime.thinkingFinish(ctx, thinking, "thinking.failed", err), agentFinalReply{}
			}
			if err := runtime.thinkingFinish(ctx, thinking, "thinking.completed", nil); err != nil {
				return err, agentFinalReply{}
			}
			return nil, final
		}
		if round >= selected.MaxTurns {
			err := fmt.Errorf("OpenAI-compatible completion exceeded turn limit")
			return runtime.thinkingFinish(ctx, thinking, "thinking.failed", err), agentFinalReply{}
		}
		if err := runtime.thinkingFinish(ctx, thinking, "thinking.completed", nil); err != nil {
			return err, agentFinalReply{}
		}
		if err := runtime.replyCancellationCheck(ctx, parent); err != nil {
			return err, agentFinalReply{}
		}
		_, err = runtime.runLispCalls(ctx, parent, selected.Ref, principal, round, callCount, reply.ToolCalls)
		if err != nil {
			return err, agentFinalReply{}
		}
		callCount += len(reply.ToolCalls)
	}
}

func (runtime *SessionEventReplyRuntime) openAICompatibleComplete(ctx dbos.Context, selected *database.WorkspaceAgentModel, request openAICompatibleRequest) (openAICompatibleMessage, error) {
	for {
		reply, err := dbos.RunAsStep(ctx, func(step context.Context) (openAICompatibleMessage, error) {
			err, apiKey := runtime.openAIAPIKey(step, selected)
			if err != nil {
				return openAICompatibleMessage{}, err
			}
			defer clear(apiKey)
			err, reply := OpenAICompatibleComplete(step, &http.Client{Timeout: time.Minute}, *selected.BaseURL, string(apiKey), request)
			return reply, err
		}, dbos.WithStepName("gatehouse.session-event-reply-completion"))
		if err == nil {
			return reply, err
		}
		var retryable *openAIRetryableError
		if !errors.As(err, &retryable) {
			return reply, err
		}
		if _, err := dbos.Sleep(ctx, retryable.RetryAfter()); err != nil {
			return openAICompatibleMessage{}, err
		}
	}
}

func (runtime *SessionEventReplyRuntime) openAIResponsesComplete(ctx dbos.Context, selected *database.WorkspaceAgentModel, request openAIResponsesRequest) (openAIResponsesResponse, error) {
	for {
		reply, err := dbos.RunAsStep(ctx, func(step context.Context) (openAIResponsesResponse, error) {
			err, apiKey := runtime.openAIAPIKey(step, selected)
			if err != nil {
				return openAIResponsesResponse{}, err
			}
			defer clear(apiKey)
			err, reply := OpenAIResponsesComplete(step, &http.Client{Timeout: time.Minute}, *selected.BaseURL, string(apiKey), request)
			return reply, err
		}, dbos.WithStepName("gatehouse.session-event-reply-responses"))
		if err == nil {
			return reply, err
		}
		var retryable *openAIRetryableError
		if !errors.As(err, &retryable) {
			return reply, err
		}
		if _, err := dbos.Sleep(ctx, retryable.RetryAfter()); err != nil {
			return openAIResponsesResponse{}, err
		}
	}
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
	err, tools, bindings, files, projectInfo, projectFiles, projectNotes, sessionNotes, values := runtime.turnEnvironment(ctx, input.Request.Ref.Session, input.Principal)
	if err != nil {
		return runtime.toolCallFinish(ctx, input, sessionToolCallExecution{Kind: "tool.failure", Output: err.Error()})
	}
	defer clearResourceValues(values)
	sessionNotes.Create = runtime.sessionNoteCreate(ctx, input.Request.Ref.Session, input.Principal)
	sessionNotes.Remove = runtime.sessionNoteRemove(ctx, input.Request.Ref.Session, input.Principal)
	if projectInfo != nil {
		projectInfo.Set = runtime.projectInfoSet(ctx, input.Request.Ref.Session, input.Principal)
	}
	if projectNotes != nil {
		projectNotes.Create = runtime.projectNoteCreate(ctx, input.Request.Ref.Session, input.Principal)
		projectNotes.Remove = runtime.projectNoteRemove(ctx, input.Request.Ref.Session, input.Principal)
	}
	call, err := diagnostics.Begin("agent.tool_call.evaluate", "")
	if err != nil {
		return runtime.toolCallFinish(ctx, input, sessionToolCallExecution{Kind: "tool.failure", Output: err.Error()})
	}
	sessionFileRead := func(id string, offset, length int64) (error, []byte) {
		err, file, _ := runtime.store.SessionFileGet(ctx, model.SessionFileRef{Session: input.Request.Ref.Session, Id: id}, input.Principal)
		if err != nil || file == nil {
			if err != nil {
				return err, nil
			}
			return fmt.Errorf("read session file: unavailable"), nil
		}
		return runtime.storage.Read(ctx, file.StorageObject.Id, offset, length)
	}
	sessionEventRead := func(id string, offset, length int64) (error, []byte) {
		err, event := runtime.store.SessionEventGet(ctx, model.SessionEventRef{Session: input.Request.Ref.Session, Id: id})
		if err != nil || event == nil {
			if err != nil {
				return err, nil
			}
			return fmt.Errorf("read session event: unavailable"), nil
		}
		return sessionEventReadRange(*event, offset, length)
	}
	modules := []lisp.HostModule{
		NewProjectModule(projectInfo, projectFiles, projectNotes),
		NewSessionModule(files, sessionFileRead, runtime.sessionFileCreate(ctx, input.Request.Ref.Session, input.Principal), sessionNotes, sessionEventRead),
		NewPolicyModule(func(description string) error { return runtime.awaitApproval(ctx, input, description) }),
		NewWebModule(),
	}
	evalErr, result := lisp.Evaluate(input.Code, lisp.EvalOptions{Prelude: agentPrelude, Bindings: bindings, SourceModules: tools, HostModules: modules})
	evalErr = call.End(evalErr)
	execution := sessionToolCallExecution{}
	if evalErr != nil {
		execution = sessionToolCallExecution{Kind: "tool.failure", Output: evalErr.Error()}
	} else {
		execution = sessionToolCallExecution{Kind: "tool.success", Output: result.String()}
	}
	return runtime.toolCallFinish(ctx, input, execution)
}

func sessionEventReadRange(event model.SessionEvent, offset, length int64) (error, []byte) {
	var output string
	var available bool
	switch event.Kind {
	case "message.text":
		output, available = event.Payload["text"].(string)
	case "tool.success", "tool.failure":
		output, available = event.Payload["output"].(string)
	}
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

func (runtime *SessionEventReplyRuntime) toolCallFinish(ctx dbos.Context, input SessionToolCallInput, execution sessionToolCallExecution) (string, error) {
	resultEvent := model.SessionEvent{
		Ref: model.SessionEventRef{Session: input.Request.Ref.Session}, Parent: &input.Request.Ref, Kind: execution.Kind, AuthorAgent: &input.Agent,
		Payload: map[string]interface{}{"name": "lisp", "call_id": input.CallID, "output": execution.Output}, CreatedAt: toolOutputCreatedAt(input.Request.CreatedAt),
	}
	err, _ := runtime.persistAgentEvent(ctx, resultEvent)
	if err != nil {
		return "", err
	}
	return execution.Output, nil
}

func (runtime *SessionEventReplyRuntime) sessionNoteCreate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) SessionNoteCreate {
	return func(title, description, body string, sensitive bool) (error, SessionNote) {
		note, err := dbos.RunAsStep(ctx, func(step context.Context) (SessionNote, error) {
			id, err := typed_id.New(typed_id.SessionNote)
			if err != nil {
				return SessionNote{}, fmt.Errorf("generate session note ID: %w", err)
			}
			err, note := runtime.store.SessionNoteCreate(step, model.SessionNote{
				Ref: model.SessionNoteRef{Session: session, Id: id}, Title: title, Description: description, Body: body, Sensitive: sensitive,
			}, principal)
			if err != nil {
				return SessionNote{}, err
			}
			return SessionNote{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Sensitive: note.Sensitive, AuthorID: note.AuthorPrincipal.Id, CreatedAt: note.CreatedAt}, nil
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
				Ref: model.SessionFileRef{Session: session, Id: fileID}, Name: name, MediaType: &mediaTypeValue,
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

func (runtime *SessionEventReplyRuntime) sessionNoteRemove(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) NoteRemove {
	return func(id string) (error, bool) {
		removed, err := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
			err, removed := runtime.store.SessionNoteRemove(step, model.SessionNoteRef{Session: session, Id: id}, principal)
			return removed, err
		}, dbos.WithStepName("gatehouse.session-tool-call-session-note-remove"))
		return err, removed
	}
}

func (runtime *SessionEventReplyRuntime) projectNoteCreate(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) ProjectNoteCreate {
	return func(title, description, body string, sensitive bool) (error, ProjectNote) {
		note, err := dbos.RunAsStep(ctx, func(step context.Context) (ProjectNote, error) {
			err, project := runtime.store.SessionProjectGet(step, session)
			if err != nil {
				return ProjectNote{}, err
			}
			if project == nil {
				return ProjectNote{}, fmt.Errorf("create project note: project is unavailable")
			}
			id, err := typed_id.New(typed_id.ProjectNote)
			if err != nil {
				return ProjectNote{}, fmt.Errorf("generate project note ID: %w", err)
			}
			err, note := runtime.store.ProjectNoteCreate(step, model.ProjectNote{
				Ref: model.ProjectNoteRef{Project: *project, Id: id}, Title: title, Description: description, Body: body, Sensitive: sensitive,
			}, principal)
			if err != nil {
				return ProjectNote{}, err
			}
			return ProjectNote{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Sensitive: note.Sensitive, AuthorID: note.AuthorPrincipal.Id, CreatedAt: note.CreatedAt}, nil
		}, dbos.WithStepName("gatehouse.session-tool-call-project-note-create"))
		return err, note
	}
}

func (runtime *SessionEventReplyRuntime) projectNoteRemove(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) NoteRemove {
	return func(id string) (error, bool) {
		removed, err := dbos.RunAsStep(ctx, func(step context.Context) (bool, error) {
			err, project := runtime.store.SessionProjectGet(step, session)
			if err != nil {
				return false, err
			}
			if project == nil {
				return false, fmt.Errorf("remove project note: project is unavailable")
			}
			err, removed := runtime.store.ProjectNoteRemove(step, model.ProjectNoteRef{Project: *project, Id: id}, principal)
			return removed, err
		}, dbos.WithStepName("gatehouse.session-tool-call-project-note-remove"))
		return err, removed
	}
}

func (runtime *SessionEventReplyRuntime) projectInfoSet(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef) ProjectInfoSet {
	return func(name, description string) (error, ProjectInfo) {
		updated, err := dbos.RunAsStep(ctx, func(step context.Context) (model.Project, error) {
			err, project := runtime.store.SessionProjectGet(step, session)
			if err != nil {
				return model.Project{}, err
			}
			if project == nil {
				return model.Project{}, fmt.Errorf("update project details: project is unavailable")
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

func (runtime *SessionEventReplyRuntime) awaitApproval(ctx dbos.Context, input SessionToolCallInput, description string) error {
	err, request := runtime.persistAgentEvent(ctx, model.SessionEvent{
		Ref: model.SessionEventRef{Session: input.Request.Ref.Session}, Parent: &input.Request.Ref, Kind: "approval.request", AuthorAgent: &input.Agent, Payload: map[string]interface{}{"description": description},
	})
	if err != nil {
		return err
	}
	handle, err := dbos.RunWorkflow(ctx, runtime.approval, SessionApprovalInput{Request: request.Ref},
		dbos.WithRunInstance(runtime),
		dbos.WithWorkflowID(sessionApprovalWorkflowID(request.Ref)),
	)
	if err != nil {
		return fmt.Errorf("start approval %q: %w", request.Ref.Id, err)
	}
	outcome, err := handle.GetResult()
	if err != nil {
		return fmt.Errorf("await approval %q: %w", request.Ref.Id, err)
	}
	if outcome == ApprovalGranted {
		return nil
	}
	if outcome == ApprovalRejected {
		return lisp.Errorf("approval rejected")
	}
	return fmt.Errorf("approval %q returned invalid outcome %q", request.Ref.Id, outcome)
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
		case "approval.approved":
			return ApprovalGranted, nil
		case "approval.rejected":
			return ApprovalRejected, nil
		default:
			return "", fmt.Errorf("approval response has invalid kind %q", event.Kind)
		}
	}, dbos.WithStepName("gatehouse.session-approval-response"))
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
		request := model.SessionEvent{
			Ref:         model.SessionEventRef{Session: parent.Session},
			Parent:      &parent,
			Kind:        "tool.request",
			AuthorAgent: &agent,
			Payload:     map[string]interface{}{"name": "lisp", "call_id": call.ID, "code": code, "reason": reason, "batch": round, "position": offset + index},
		}
		err, stored := runtime.persistAgentEvent(ctx, request)
		if err != nil {
			return nil, err
		}
		inputs[index] = SessionToolCallInput{Request: stored, Agent: agent, Principal: principal, CallID: call.ID, Code: code, Reason: reason}
	}
	handles := make([]dbos.WorkflowHandle[string], len(inputs))
	for index, input := range inputs {
		handle, err := dbos.RunWorkflow(ctx, runtime.toolCall, input,
			dbos.WithRunInstance(runtime),
			dbos.WithWorkflowID(sessionToolCallWorkflowID(input.Request.Ref)),
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

const openAISystemPrompt = `# Role

You are an agent that completes user requests using authorized workspace capabilities. Use available tools and resources to perform the work, not merely describe how it could be done. Verify results before reporting completion, and continue working when verification shows the result is missing, invalid, or incomplete. Communicate clear user-facing results without mentioning your tools or implementation details.

# Tools

You have one tool, ` + "`lisp`" + `. The Lisp environment contains all authorized workspace capabilities and resources. It is a custom Lisp dialect, not Common Lisp or Scheme. Use its discovery bindings to learn available capabilities.

Session capabilities are imported from ` + "`@native:gatehouse/session/v1`" + `, project capabilities from ` + "`@native:gatehouse/project/v1`" + `, and web capabilities from ` + "`@native:gatehouse/web/v1`" + `. Session file attachments are available through ` + "`session/files/list`" + `, ` + "`session/files/info`" + `, and ` + "`session/files/read`" + `. Read message text or tool output by event ID through ` + "`session/events/read`" + ` using a byte offset and length. Create a file with ` + "`session/files/create`" + ` using its name, media type, and a sequence of Bytes chunks; it returns the file ID to include in your final response attachments. Shared session notes are available through ` + "`session/notes/list`" + `, ` + "`session/notes/read`" + `, ` + "`session/notes/create`" + `, and ` + "`session/notes/remove`" + `. Create notes with title, description, and Markdown body strings; description and body may be empty. Project metadata is available through ` + "`project/info/get`" + `, which returns ` + "`null`" + ` when no authorized project is linked, and ` + "`project/info/set`" + `, which replaces its name and description. Its files are available through ` + "`project/files/list`" + `, ` + "`project/files/info`" + `, and ` + "`project/files/read`" + `, and its notes through ` + "`project/notes/list`" + `, ` + "`project/notes/read`" + `, ` + "`project/notes/create`" + `, and ` + "`project/notes/remove`" + `. Search the web through ` + "`web/search`" + ` using a non-sensitive query, and fetch raw page bodies through ` + "`web/fetch`" + ` using a public HTTPS URL. Inspect project, file, and note metadata first, then read only the ranges needed to complete the request.

## Large Tool Results

In the transcript, large tool results may be represented by a truncated preview of the full data. Write Lisp programs that return concise checks, summaries, or filtered results instead of raw data. Test transformations on small fixtures and make heavy use of ` + "`assert`" + ` for validation. If necessary, use ` + "`session/events/read`" + ` to inspect targeted ranges of truncated results. When the final result is large, create and attach a session file instead of dumping it into tool output.

Examples:

- ` + "`(help/env) ; List all bindings.`" + `
- ` + "`(help/env \"prefix\") ; List bindings matching a prefix.`" + `
- ` + "`(help/search \"term\") ; Search documented capabilities.`" + `
- ` + "`(help 'binding) ; Inspect a binding.`" + `
- ` + "`(let ((factor 2) (value 21)) (* factor value)) ; Uses lexical bindings and returns 42.`" + `
- ` + "`(list/map (fn (number) (* number number)) (list 1 2 3)) ; Returns (1 4 9).`" + `
- ` + "`(fn/apply + (list 1 2 3)) ; Calls a function with values from a list.`" + `

# Instructions

Treat tool failures as feedback. Correct and retry when the request remains answerable. Return unexecuted code only when the user explicitly asks for code rather than its result. When the work is complete, return the required structured response with user-facing text and the IDs of any session files to attach.`

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

func (runtime *SessionEventReplyRuntime) openAIResponsesReply(ctx dbos.Context, parent model.SessionEventRef, selected *database.WorkspaceAgentModel, principal model.PrincipalRef, reasoningEffort string) (error, agentFinalReply) {
	var reasoning *openAIResponsesReasoning
	if reasoningEffort != "" {
		reasoning = &openAIResponsesReasoning{Effort: reasoningEffort}
	}
	callCount := 0
	for round := 0; ; round++ {
		if err := runtime.replyCancellationCheck(ctx, parent); err != nil {
			return err, agentFinalReply{}
		}
		err, messages := runtime.agentContext(ctx, parent, selected)
		if err != nil {
			return err, agentFinalReply{}
		}
		input := openAIResponsesInput(messages)
		err, thinking := runtime.thinkingStart(ctx, parent, selected.Ref, round)
		if err != nil {
			return err, agentFinalReply{}
		}
		reply, err := runtime.openAIResponsesComplete(ctx, selected, openAIResponsesRequest{
			Model: selected.Model, Instructions: openAISystemPromptFor(selected), Input: input, Tools: []openAIResponsesTool{openAIResponsesLispTool()}, Text: openAIResponsesFinalReplyText(), ParallelToolCalls: true, Reasoning: reasoning, MaxOutputTokens: selected.MaxOutputTokens,
		})
		if err != nil {
			return runtime.thinkingFinish(ctx, thinking, "thinking.failed", err), agentFinalReply{}
		}
		calls := make([]openAICompatibleToolCall, 0, 1)
		outputs := make([]openAIResponsesOutput, len(reply.Output))
		for index, raw := range reply.Output {
			var output openAIResponsesOutput
			if err := json.Unmarshal(raw, &output); err != nil {
				err := fmt.Errorf("decode OpenAI Responses output: %w", err)
				logInvalidOpenAIResponsesFinalReply(selected.ProviderID, parent, input, reply.Output)
				return runtime.thinkingFinish(ctx, thinking, "thinking.failed", err), agentFinalReply{}
			}
			outputs[index] = output
			switch output.Type {
			case "function_call":
				call := openAICompatibleToolCall{ID: output.CallID, Type: "function"}
				call.Function.Name = output.Name
				call.Function.Arguments = output.Arguments
				calls = append(calls, call)
			}
		}
		if len(calls) == 0 {
			final, err := openAIResponsesFinalReply(outputs)
			if err != nil {
				logInvalidOpenAIResponsesFinalReply(selected.ProviderID, parent, input, reply.Output)
				return runtime.thinkingFinish(ctx, thinking, "thinking.failed", err), agentFinalReply{}
			}
			if err := runtime.thinkingFinish(ctx, thinking, "thinking.completed", nil); err != nil {
				return err, agentFinalReply{}
			}
			return nil, final
		}
		if round >= selected.MaxTurns {
			err := fmt.Errorf("OpenAI Responses exceeded turn limit")
			return runtime.thinkingFinish(ctx, thinking, "thinking.failed", err), agentFinalReply{}
		}
		if err := runtime.thinkingFinish(ctx, thinking, "thinking.completed", nil); err != nil {
			return err, agentFinalReply{}
		}
		if err := runtime.replyCancellationCheck(ctx, parent); err != nil {
			return err, agentFinalReply{}
		}
		_, err = runtime.runLispCalls(ctx, parent, selected.Ref, principal, round, callCount, calls)
		if err != nil {
			return err, agentFinalReply{}
		}
		callCount += len(calls)
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

func (runtime *SessionEventReplyRuntime) persistAgentEvent(ctx dbos.Context, event model.SessionEvent) (error, model.SessionEvent) {
	stored, err := dbos.RunAsTransaction(ctx, runtime.dataSource, func(step context.Context, transaction dbos.Tx) (model.SessionEvent, error) {
		id, err := typed_id.New(typed_id.SessionEvent)
		if err != nil {
			return model.SessionEvent{}, fmt.Errorf("generate session event ID: %w", err)
		}
		event.Ref.Id = id
		err, stored := runtime.store.SessionEventCreateInTransaction(step, transaction, event)
		return stored, err
	}, dbos.WithStepName("gatehouse.session-event-agent-persist"))
	return err, stored
}

func (runtime *SessionEventReplyRuntime) replyCancellationRequested(ctx context.Context, parent model.SessionEventRef) (error, *model.SessionEvent) {
	return runtime.store.SessionEventChildGet(ctx, parent, "cancel.request")
}

func (runtime *SessionEventReplyRuntime) replyCancellationCheck(ctx dbos.Context, parent model.SessionEventRef) error {
	err, cancelled := runtime.replyCancellationComplete(ctx, parent)
	if err != nil {
		return err
	}
	if !cancelled {
		return nil
	}
	return sessionReplyCancelled{}
}

func (runtime *SessionEventReplyRuntime) replyCancellationComplete(ctx dbos.Context, parent model.SessionEventRef) (error, bool) {
	request, err := dbos.RunAsStep(ctx, func(step context.Context) (*model.SessionEvent, error) {
		err, request := runtime.replyCancellationRequested(step, parent)
		return request, err
	}, dbos.WithStepName("gatehouse.session-event-reply-cancellation-check"))
	if err != nil {
		return err, false
	}
	if request == nil {
		return nil, false
	}
	event := model.SessionEvent{
		Ref: model.SessionEventRef{Session: parent.Session}, Parent: &request.Ref, Kind: "cancel.success", AuthorPrincipal: request.AuthorPrincipal,
		Payload: map[string]interface{}{}, CreatedAt: eventTerminalCreatedAt(request.CreatedAt),
	}
	err, _ = runtime.persistAgentEvent(ctx, event)
	return err, err == nil
}

func (runtime *SessionEventReplyRuntime) thinkingStart(ctx dbos.Context, parent model.SessionEventRef, agent model.WorkspaceAgentRef, turn int) (error, model.SessionEvent) {
	event := model.SessionEvent{
		Ref: model.SessionEventRef{Session: parent.Session}, Parent: &parent, Kind: "thinking.started", AuthorAgent: &agent,
		Payload: map[string]interface{}{"turn": turn},
	}
	return runtime.persistAgentEvent(ctx, event)
}

func (runtime *SessionEventReplyRuntime) thinkingFinish(ctx dbos.Context, started model.SessionEvent, kind string, completionErr error) error {
	event := model.SessionEvent{
		Ref: model.SessionEventRef{Session: started.Ref.Session}, Parent: &started.Ref, Kind: kind, AuthorAgent: started.AuthorAgent,
		Payload: map[string]interface{}{}, CreatedAt: eventTerminalCreatedAt(started.CreatedAt),
	}
	err, _ := runtime.persistAgentEvent(ctx, event)
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

func (runtime *SessionEventReplyRuntime) turnEnvironment(ctx context.Context, session model.SessionRef, principal model.PrincipalRef) (error, []lisp.SourceModule, []lisp.Binding, []File, *ProjectInfo, *ProjectFiles, *ProjectNotes, *SessionNotes, map[model.ResourceRef][]byte) {
	workspace := session.Workspace
	err, configuredTools := runtime.store.WorkspaceToolsGet(ctx, workspace, principal)
	if err != nil {
		return err, nil, nil, nil, nil, nil, nil, nil, nil
	}
	err, configuredResources := runtime.store.WorkspaceResourcesGet(ctx, workspace, principal)
	if err != nil {
		return err, nil, nil, nil, nil, nil, nil, nil, nil
	}
	err, values := runtime.resolveResources(ctx, configuredResources)
	if err != nil {
		return err, nil, nil, nil, nil, nil, nil, nil, nil
	}
	tools := make([]lisp.SourceModule, 0, len(configuredTools))
	for _, configured := range configuredTools {
		err, source := readToolSource(configured.Source)
		if err != nil {
			clearResourceValues(values)
			return fmt.Errorf("read tool %q: %w", configured.Ref.Id, err), nil, nil, nil, nil, nil, nil, nil, nil
		}
		tools = append(tools, lisp.SourceModule{ID: configured.Ref.Id, Source: source})
	}
	bindings := make([]lisp.Binding, 0, len(configuredResources))
	for _, configured := range configuredResources {
		value := lisp.Bytes(values[configured.Ref])
		if configured.Secret {
			value = lisp.MarkSecret(value)
		}
		bindings = append(bindings, lisp.Binding{Name: "resource/" + configured.Ref.Id, Value: value})
	}
	err, summaries := runtime.store.SessionFilesGet(ctx, session)
	if err != nil {
		clearResourceValues(values)
		return err, nil, nil, nil, nil, nil, nil, nil, nil
	}
	files := make([]File, 0, len(summaries))
	for _, file := range summaries {
		files = append(files, File{ID: file.ID, Name: file.Name, MediaType: file.MediaType, Size: file.Size, Fingerprint: file.Fingerprint})
	}
	err, noteSummaries := runtime.store.SessionNotesGet(ctx, session, principal)
	if err != nil {
		clearResourceValues(values)
		return err, nil, nil, nil, nil, nil, nil, nil, nil
	}
	sessionNotes := &SessionNotes{Notes: make([]SessionNote, 0, len(noteSummaries))}
	for _, note := range noteSummaries {
		sessionNotes.Notes = append(sessionNotes.Notes, SessionNote{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Sensitive: note.Sensitive, AuthorID: note.AuthorPrincipal.Id, AuthorName: note.AuthorName, CreatedAt: note.CreatedAt})
	}
	sessionNotes.Read = func(id string, offset, length int64) (error, []byte, bool) {
		err, detail := runtime.store.SessionNoteGet(ctx, model.SessionNoteRef{Session: session, Id: id}, principal)
		if err != nil || detail == nil {
			if err != nil {
				return err, nil, false
			}
			return fmt.Errorf("read session note: unavailable"), nil, false
		}
		body := []byte(detail.Note.Body)
		if offset > int64(len(body)) {
			return fmt.Errorf("read session note: offset is unavailable"), nil, false
		}
		end := offset + length
		if end > int64(len(body)) {
			end = int64(len(body))
		}
		return nil, append([]byte(nil), body[offset:end]...), detail.Note.Sensitive
	}
	err, project := runtime.store.SessionProjectGet(ctx, session)
	if err != nil {
		clearResourceValues(values)
		return err, nil, nil, nil, nil, nil, nil, nil, nil
	}
	var projectInfo *ProjectInfo
	var projectFiles *ProjectFiles
	var projectNotes *ProjectNotes
	if project != nil {
		err, authorized := runtime.store.ProjectGet(ctx, *project, principal)
		if err != nil {
			clearResourceValues(values)
			return err, nil, nil, nil, nil, nil, nil, nil, nil
		}
		if authorized != nil {
			projectInfo = &ProjectInfo{Name: authorized.Name, Description: authorized.Description, CreatedAt: authorized.CreatedAt}
			err, summaries := runtime.store.ProjectFilesGet(ctx, *project, principal)
			if err != nil {
				clearResourceValues(values)
				return err, nil, nil, nil, nil, nil, nil, nil, nil
			}
			projectFiles = &ProjectFiles{Files: make([]File, 0, len(summaries))}
			for _, file := range summaries {
				projectFiles.Files = append(projectFiles.Files, File{ID: file.ID, Name: file.Name, MediaType: file.MediaType, Size: file.Size, Fingerprint: file.Fingerprint})
			}
			projectFiles.Read = func(id string, offset, length int64) (error, []byte) {
				err, file, object := runtime.store.ProjectFileGet(ctx, model.ProjectFileRef{Project: *project, Id: id}, principal)
				if err != nil || file == nil || object == nil {
					if err != nil {
						return err, nil
					}
					return fmt.Errorf("read project file: unavailable"), nil
				}
				return runtime.storage.Read(ctx, object.ID, offset, length)
			}
			err, noteSummaries := runtime.store.ProjectNotesGet(ctx, *project, principal)
			if err != nil {
				clearResourceValues(values)
				return err, nil, nil, nil, nil, nil, nil, nil, nil
			}
			projectNotes = &ProjectNotes{Notes: make([]ProjectNote, 0, len(noteSummaries))}
			for _, note := range noteSummaries {
				projectNotes.Notes = append(projectNotes.Notes, ProjectNote{ID: note.Ref.Id, Title: note.Title, Description: note.Description, Sensitive: note.Sensitive, AuthorID: note.AuthorPrincipal.Id, AuthorName: note.AuthorName, CreatedAt: note.CreatedAt})
			}
			projectNotes.Read = func(id string, offset, length int64) (error, []byte, bool) {
				err, detail := runtime.store.ProjectNoteGet(ctx, model.ProjectNoteRef{Project: *project, Id: id}, principal)
				if err != nil || detail == nil {
					if err != nil {
						return err, nil, false
					}
					return fmt.Errorf("read project note: unavailable"), nil, false
				}
				body := []byte(detail.Note.Body)
				if offset > int64(len(body)) {
					return fmt.Errorf("read project note: offset is unavailable"), nil, false
				}
				end := offset + length
				if end > int64(len(body)) {
					end = int64(len(body))
				}
				return nil, append([]byte(nil), body[offset:end]...), detail.Note.Sensitive
			}
		}
	}
	return nil, tools, bindings, files, projectInfo, projectFiles, projectNotes, sessionNotes, values
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
			event    model.SessionEvent
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
				event    model.SessionEvent
			}{position: int(position), call: call, event: event})
			callBatches[event.Ref.Id] = key
		}
	}
	for _, event := range events {
		switch event.Kind {
		case "message.text":
			message, text, ok := transcriptMessageContent(event)
			if !ok {
				continue
			}
			if event.AuthorPrincipal != nil {
				messages = append(messages, openAICompatibleMessage{Role: "user", Content: text, Message: message})
			} else if event.AuthorAgent != nil {
				messages = append(messages, openAICompatibleMessage{Role: "assistant", Content: text, Message: message})
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
			status := "success"
			if event.Kind == "tool.failure" {
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

func sessionNameWorkflowID(session model.SessionRef) string {
	return "session-name:" + sessionEventReplyPartition(session)
}

func sessionToolCallWorkflowID(request model.SessionEventRef) string {
	return "session-tool-call:" + request.Id
}

func sessionApprovalWorkflowID(request model.SessionEventRef) string {
	return "session-approval:" + request.Id
}

func sessionEventReplyPartition(session model.SessionRef) string {
	return session.Workspace.Id + "/" + session.Id
}
