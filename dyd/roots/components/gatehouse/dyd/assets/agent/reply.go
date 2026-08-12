package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/lisp"
	"gatehouse/model"
)

const sessionEventReplyQueue = "gatehouse.session-event-replies"

type SessionEventReplyInput struct {
	Event model.SessionEventRef
}

type SessionEventReplyRuntime struct {
	store *database.Store
	keyring *keychain.Keyring
	dbos  dbos.Context
	queue dbos.Queue
}

func NewSessionEventReplyRuntime(ctx dbos.Context, store *database.Store, keyring *keychain.Keyring) (error, *SessionEventReplyRuntime) {
	queue, err := dbos.RegisterQueue(ctx, sessionEventReplyQueue,
		dbos.WithPartitionQueue(),
		dbos.WithGlobalConcurrency(1),
	)
	if err != nil {
		return fmt.Errorf("register session event reply queue: %w", err), nil
	}
	runtime := &SessionEventReplyRuntime{store: store, keyring: keyring, dbos: ctx, queue: queue}
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
		err, message := runtime.store.SessionEventGet(step, input.Event)
		if err != nil {
			return model.SessionEvent{}, err
		}
		if message == nil {
			return model.SessionEvent{}, fmt.Errorf("reply to session event %q: event not found", input.Event.Id)
		}
		err, events := runtime.store.SessionEventsGet(step, input.Event.Session)
		if err != nil {
			return model.SessionEvent{}, err
		}
		err, messages := openAICompatibleMessages(events)
		if err != nil {
			return model.SessionEvent{}, err
		}
		if len(messages) == 0 {
			return model.SessionEvent{}, fmt.Errorf("reply to session event %q: message has no text", input.Event.Id)
		}
		var text string
		switch selected.Protocol {
		case "builtin":
			err, text = BuiltinReply(selected.Model, selected.Parameters)
		case "openai-chat-completions", "openai-responses":
			if message.AuthorPrincipal == nil {
				return model.SessionEvent{}, fmt.Errorf("reply to session event %q: Lisp authorization requires a principal author", input.Event.Id)
			}
			err, text = runtime.openAIReply(step, input.Event, selected, messages, *message.AuthorPrincipal)
		default:
			err = fmt.Errorf("unsupported provider protocol %q", selected.Protocol)
		}
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

func (runtime *SessionEventReplyRuntime) openAIReply(ctx context.Context, parent model.SessionEventRef, selected *database.WorkspaceAgentModel, messages []openAICompatibleMessage, principal model.PrincipalRef) (error, string) {
	if selected.BaseURL == nil || selected.Keychain == nil || selected.APIKey == nil {
		return fmt.Errorf("reply with OpenAI-compatible provider %q: missing credentials", selected.ProviderID), ""
	}
	err, tools := runtime.store.WorkspaceToolsGet(ctx, parent.Session.Workspace, principal)
	if err != nil {
		return err, ""
	}
	err, configuredResources := runtime.store.WorkspaceResourcesGet(ctx, parent.Session.Workspace, principal)
	if err != nil {
		return err, ""
	}
	resources := make([]config.Resource, 0, len(configuredResources))
	for _, configured := range configuredResources {
		resources = append(resources, config.Resource{WorkspaceID: configured.Ref.Workspace.Id, ID: configured.Ref.Id, Source: configured.Source, Secret: configured.Secret, Enabled: configured.Enabled})
	}
	err, values := runtime.resolveResources(ctx, resources)
	if err != nil {
		return err, ""
	}
	defer clearResourceValues(values)
	turnTools := make([]lisp.TurnTool, 0, len(tools))
	for _, tool := range tools {
		err, source := readToolSource(tool.Source)
		if err != nil {
			return fmt.Errorf("read tool %q: %w", tool.Ref.Id, err), ""
		}
		turnTools = append(turnTools, lisp.TurnTool{ID: tool.Ref.Id, Source: source})
	}
	turnResources := make([]lisp.TurnResource, 0, len(resources))
	for _, configured := range resources {
		ref := model.ResourceRef{Workspace: model.WorkspaceRef{Id: configured.WorkspaceID}, Id: configured.ID}
		turnResources = append(turnResources, lisp.TurnResource{ID: configured.ID, Value: values[ref], Secret: configured.Secret})
	}

	err, encrypted := keychain.ParseKey(*selected.APIKey)
	if err != nil {
		return fmt.Errorf("parse API key for provider %q: %w", selected.ProviderID, err), ""
	}
	err, keys := runtime.keyring.Get(ctx, []model.KeychainRef{*selected.Keychain})
	if err != nil {
		return fmt.Errorf("get keychain for provider %q: %w", selected.ProviderID, err), ""
	}
	key := keys[*selected.Keychain]
	decryptedErr, apiKey := keychain.Open(key, []byte("gh=v1|agent-provider="+selected.ProviderID), encrypted)
	clear(key)
	clear(keys)
	if decryptedErr != nil {
		return fmt.Errorf("decrypt API key for provider %q: %w", selected.ProviderID, decryptedErr), ""
	}
	defer clear(apiKey)

	err, reasoningEffort := openAICompatibleReasoningEffort(selected.Parameters)
	if err != nil {
		return err, ""
	}
	if selected.Protocol == "openai-responses" {
		return runtime.openAIResponsesReply(ctx, parent, selected, messages, turnTools, turnResources, string(apiKey), reasoningEffort)
	}
	requestMessages := append([]openAICompatibleMessage{{Role: "system", Content: openAISystemPrompt}}, messages...)
	for index := 0; index < 16; index++ {
		err, reply := OpenAICompatibleComplete(ctx, &http.Client{Timeout: time.Minute}, *selected.BaseURL, string(apiKey), openAICompatibleRequest{
			Model: selected.Model, Messages: requestMessages, Tools: []openAICompatibleTool{openAICompatibleLispTool()}, ReasoningEffort: reasoningEffort,
		})
		if err != nil {
			return err, ""
		}
		if len(reply.ToolCalls) == 0 {
			if strings.TrimSpace(reply.Content) == "" {
				return fmt.Errorf("OpenAI-compatible completion returned no message"), ""
			}
			return nil, reply.Content
		}
		if len(reply.ToolCalls) != 1 || reply.ToolCalls[0].Type != "function" || reply.ToolCalls[0].Function.Name != "lisp" || reply.ToolCalls[0].ID == "" {
			return fmt.Errorf("OpenAI-compatible completion requested an unsupported tool call"), ""
		}
		call := reply.ToolCalls[0]
		output, err := runtime.runLispCall(ctx, parent, selected, index, call.ID, call.Function.Arguments, turnTools, turnResources)
		if err != nil {
			return err, ""
		}
		requestMessages = append(requestMessages, reply, openAICompatibleMessage{Role: "tool", ToolCallID: call.ID, Content: output})
	}
	return fmt.Errorf("OpenAI-compatible completion exceeded Lisp tool-call limit"), ""
}

func (runtime *SessionEventReplyRuntime) runLispCall(ctx context.Context, parent model.SessionEventRef, selected *database.WorkspaceAgentModel, index int, callID, encodedArguments string, tools []lisp.TurnTool, resources []lisp.TurnResource) (string, error) {
		var arguments struct {
			Code   string `json:"code"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(encodedArguments), &arguments); err != nil || strings.TrimSpace(arguments.Code) == "" || strings.TrimSpace(arguments.Reason) == "" {
			return "", fmt.Errorf("OpenAI requested invalid Lisp arguments")
		}
		callEvent := model.SessionEvent{
			Ref: model.SessionEventRef{Session: parent.Session, Id: sessionEventReplyChildID(parent, "tool.call", index)}, Parent: &parent, Kind: "tool.call", AuthorAgent: &selected.Ref,
			Payload: map[string]interface{}{"name": "lisp", "call_id": callID, "code": arguments.Code, "reason": arguments.Reason},
		}
		err, storedCall := runtime.sessionEventGetOrCreate(ctx, callEvent)
		if err != nil {
			return "", err
		}
		evalErr, result := lisp.RunTurn(arguments.Code, tools, resources)
		kind, output := "tool.result", ""
		if evalErr != nil {
			kind, output = "tool.failed", evalErr.Error()
		} else {
			output = result.String()
		}
		resultEvent := model.SessionEvent{
			Ref: model.SessionEventRef{Session: parent.Session, Id: sessionEventReplyChildID(parent, kind, index)}, Parent: &storedCall.Ref, Kind: kind, AuthorAgent: &selected.Ref,
			Payload: map[string]interface{}{"name": "lisp", "call_id": callID, "output": output}, CreatedAt: toolOutputCreatedAt(storedCall.CreatedAt),
		}
		if err, _ := runtime.sessionEventGetOrCreate(ctx, resultEvent); err != nil {
			return "", err
		}
		return output, nil
}

const openAISystemPrompt = "You have one tool, lisp. The Lisp environment contains all other tools available to you. Write Lisp programs to use them. This is a custom Lisp environment, not Common Lisp or Scheme. Do not assume language features or builtin names. A tool failure is intermediate feedback: correct the Lisp and try again when the request remains answerable. Do not make a failed exploration your final answer when you can retry it. Do not mention the underlying tool, programming language, or implementation details to users; describe capabilities and results instead.\n\nExample programs:\n\n- (help/env) ; List all bindings.\n- (help/env \"prefix\") ; List bindings matching a prefix.\n- (help/search \"term\") ; Search documented capabilities.\n- (help 'binding) ; Inspect a binding.\n- (let ((factor 2) (value 21)) (* factor value)) ; Uses lexical bindings and returns 42.\n- (list/map (fn (number) (* number number)) (list 1 2 3)) ; Returns (1 4 9)."

func (runtime *SessionEventReplyRuntime) openAIResponsesReply(ctx context.Context, parent model.SessionEventRef, selected *database.WorkspaceAgentModel, messages []openAICompatibleMessage, tools []lisp.TurnTool, resources []lisp.TurnResource, apiKey, reasoningEffort string) (error, string) {
	input := openAIResponsesInput(messages)
	var reasoning *openAIResponsesReasoning
	if reasoningEffort != "" {
		reasoning = &openAIResponsesReasoning{Effort: reasoningEffort}
	}
	for index := 0; index < 16; index++ {
		err, reply := OpenAIResponsesComplete(ctx, &http.Client{Timeout: time.Minute}, *selected.BaseURL, apiKey, openAIResponsesRequest{
			Model: selected.Model, Instructions: openAISystemPrompt, Input: input, Tools: []openAIResponsesTool{openAIResponsesLispTool()}, ParallelToolCalls: false, Reasoning: reasoning,
		})
		if err != nil {
			return err, ""
		}
		input = append(input, reply.Output...)
		calls := make([]openAIResponsesOutput, 0, 1)
		text := ""
		for _, raw := range reply.Output {
			var output openAIResponsesOutput
			if err := json.Unmarshal(raw, &output); err != nil {
				return fmt.Errorf("decode OpenAI Responses output: %w", err), ""
			}
			switch output.Type {
			case "function_call":
				calls = append(calls, output)
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
				return fmt.Errorf("OpenAI Responses returned no message"), ""
			}
			return nil, text
		}
		if len(calls) != 1 || calls[0].Name != "lisp" || calls[0].CallID == "" {
			return fmt.Errorf("OpenAI Responses requested an unsupported tool call"), ""
		}
		call := calls[0]
		output, err := runtime.runLispCall(ctx, parent, selected, index, call.CallID, call.Arguments, tools, resources)
		if err != nil {
			return err, ""
		}
		input = append(input, openAIResponsesFunctionOutput(call.CallID, output))
	}
	return fmt.Errorf("OpenAI Responses exceeded Lisp tool-call limit"), ""
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

func (runtime *SessionEventReplyRuntime) resolveResources(ctx context.Context, resources []config.Resource) (error, map[model.ResourceRef][]byte) {
	values := make(map[model.ResourceRef][]byte, len(resources))
	type encryptedResource struct {
		resource  config.Resource
		encrypted keychain.Encrypted
	}
	encryptedResources := make([]encryptedResource, 0)
	references := make([]model.KeychainRef, 0)
	for _, configured := range resources {
		ref := model.ResourceRef{Workspace: model.WorkspaceRef{Id: configured.WorkspaceID}, Id: configured.ID}
		switch {
		case strings.HasPrefix(configured.Source, "file:"):
			path := strings.TrimPrefix(configured.Source, "file:")
			if path == "" {
				return fmt.Errorf("resolve resource %q: file source path is empty", configured.ID), nil
			}
			value, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("resolve resource %q from file %q: %w", configured.ID, path, err), nil
			}
			values[ref] = value
		case strings.HasPrefix(configured.Source, "env:"):
			name := strings.TrimPrefix(configured.Source, "env:")
			if name == "" {
				return fmt.Errorf("resolve resource %q: environment source name is empty", configured.ID), nil
			}
			value, ok := os.LookupEnv(name)
			if !ok {
				return fmt.Errorf("resolve resource %q: environment variable %q is not set", configured.ID, name), nil
			}
			values[ref] = []byte(value)
		case strings.HasPrefix(configured.Source, "gh-enc:"):
			err, encrypted := keychain.ParseResource(configured.Source)
			if err != nil {
				return fmt.Errorf("resolve resource %q: %w", configured.ID, err), nil
			}
			encryptedResources = append(encryptedResources, encryptedResource{resource: configured, encrypted: encrypted})
			references = append(references, *encrypted.Key)
		default:
			return fmt.Errorf("resolve resource %q: unsupported source %q", configured.ID, configured.Source), nil
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
			return fmt.Errorf("resolve resource %q: keychain key %q version %d is unavailable", configured.resource.ID, configured.encrypted.Key.Id, configured.encrypted.Key.Version), nil
		}
		valueErr, value := keychain.Open(key, []byte("gh=v1|workspace="+configured.resource.WorkspaceID+"|resource="+configured.resource.ID), configured.encrypted)
		if valueErr != nil {
			return fmt.Errorf("resolve resource %q: %w", configured.resource.ID, valueErr), nil
		}
		ref := model.ResourceRef{Workspace: model.WorkspaceRef{Id: configured.resource.WorkspaceID}, Id: configured.resource.ID}
		values[ref] = value
	}
	return nil, values
}

func openAICompatibleMessages(events []model.SessionEvent) (error, []openAICompatibleMessage) {
	messages := make([]openAICompatibleMessage, 0, len(events))
	calls := map[string]openAICompatibleToolCall{}
	for _, event := range events {
		if event.Kind != "tool.call" || event.AuthorAgent == nil {
			continue
		}
		call, err := openAICompatibleStoredToolCall(event)
		if err != nil {
			return err, nil
		}
		calls[event.Ref.Id] = call
	}
	for _, event := range events {
		switch event.Kind {
		case "message.text":
			text, ok := event.Payload["text"].(string)
			if !ok || strings.TrimSpace(text) == "" {
				continue
			}
			if event.AuthorPrincipal != nil {
				messages = append(messages, openAICompatibleMessage{Role: "user", Content: text})
			} else if event.AuthorAgent != nil {
				messages = append(messages, openAICompatibleMessage{Role: "assistant", Content: text})
			}
		case "tool.call":
			if event.AuthorAgent == nil {
				continue
			}
			call := calls[event.Ref.Id]
			messages = append(messages, openAICompatibleMessage{Role: "assistant", ToolCalls: []openAICompatibleToolCall{call}})
		case "tool.result", "tool.failed":
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
	callTime, err := time.Parse("2006-01-02T15:04:05.000Z", callCreatedAt)
	if err != nil {
		return time.Now().UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	minimum := callTime.Add(time.Millisecond)
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
	sum := sha256.Sum256([]byte(event.Session.Workspace.Id + "\x00" + event.Session.Id + "\x00" + event.Id + "\x00" + suffix))
	value := sum[:16]
	value[6] = value[6]&0x0f | 0x50
	value[8] = value[8]&0x3f | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}
