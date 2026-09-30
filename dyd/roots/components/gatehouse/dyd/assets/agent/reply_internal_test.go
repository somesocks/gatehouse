package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gatehouse/database"
	"gatehouse/lisp"
	"gatehouse/model"
)

func TestWatchCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	requested := make(chan struct{})
	stop := watchCancellation(func(context.Context) bool {
		select {
		case <-requested:
			return true
		default:
			return false
		}
	}, cancel)
	defer stop()
	close(requested)
	select {
	case <-ctx.Done():
		if ctx.Err() != context.Canceled {
			t.Fatalf("cancellation context error = %v", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation watcher did not cancel the evaluation context")
	}
}

func TestSessionToolCallInputContainsOnlyRequestReference(t *testing.T) {
	encoded, err := json.Marshal(SessionToolCallInput{Request: model.SessionEventRef{
		Session: model.SessionRef{Workspace: model.WorkspaceRef{Id: "wsp_00000000000000000000000000"}, Id: "ses_00000000000000000000000000"},
		Id:      "sev_00000000000000000000000000",
	}})
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	if len(value) != 1 || value["Request"] == nil {
		t.Fatalf("tool workflow input = %s, want only Request", encoded)
	}
}

func TestOpenAISystemPromptFor(t *testing.T) {
	customPrompt := "Custom instructions."
	emptyPrompt := ""
	selected := &database.WorkspaceAgentModel{Alias: "luna-high"}
	sharedPrompt := openAISystemPromptFor(selected, false)
	if !strings.HasPrefix(sharedPrompt, "# Role\n\nYour name is @luna-high.\nYou are an agent in a multi-user, multi-bot chat.\nYour job is to handle user requests directed at you (but not others).\n") || !strings.Contains(sharedPrompt, "# Tools\n") {
		t.Fatalf("system prompt does not render the selected agent's role: %q", sharedPrompt)
	}
	for _, instruction := range []string{"You are persistent, honest, and thorough.", "You always verify your work before replying.", "You do not lie about completing a task or give false information."} {
		if !strings.Contains(sharedPrompt, instruction) {
			t.Fatalf("system prompt lost role instruction %q", instruction)
		}
	}
	other := openAISystemPromptFor(&database.WorkspaceAgentModel{Alias: "team/sol"}, false)
	if !strings.Contains(other, "Your name is @team/sol.") || strings.Contains(other, "@luna-high") || other == sharedPrompt {
		t.Fatalf("system prompt does not use the other agent's handle: %q", other)
	}
	for name, prompt := range map[string]*string{
		"default": nil,
		"custom":  &customPrompt,
		"empty":   &emptyPrompt,
	} {
		t.Run(name, func(t *testing.T) {
			got := openAISystemPromptFor(&database.WorkspaceAgentModel{Alias: "luna-high", SystemPrompt: prompt}, false)
			if name == "default" && got != sharedPrompt {
				t.Fatalf("system prompt = %q, want shared prompt", got)
			}
			if name == "custom" && got != sharedPrompt+"\n\nCustom instructions." {
				t.Fatalf("system prompt = %q, want shared prompt plus custom instructions", got)
			}
			if name == "empty" && got != sharedPrompt {
				t.Fatalf("system prompt = %q, want shared prompt", got)
			}
		})
	}
	projectPrompt := openAISystemPromptFor(selected, true)
	if strings.Contains(sharedPrompt, "project/") || !strings.Contains(sharedPrompt, "session/*") || !strings.Contains(projectPrompt, "project/*") || !strings.Contains(projectPrompt, "session/*") {
		t.Fatalf("prompt integrations do not match the chat's project link")
	}
	if mcmtrProfileFingerprint(selected, true) == mcmtrProfileFingerprint(selected, false) {
		t.Fatal("context fingerprints for project and ordinary chats match")
	}
}

func TestOpenAIFinalReply(t *testing.T) {
	arguments, err := json.Marshal(map[string]interface{}{"text": "Done.", "attachments": []string{"sfi_01arz3ndektsv4rrffq69g5fav"}})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := openAIFinalReply(string(arguments))
	if err != nil || reply.Text != "Done." || len(reply.Attachments) != 1 || reply.Attachments[0] != "sfi_01arz3ndektsv4rrffq69g5fav" {
		t.Fatalf("openAIFinalReply() = (%#v, %v)", reply, err)
	}
	if _, err := openAIFinalReply(`{"text":"Done.","attachments":null}`); err == nil {
		t.Fatal("openAIFinalReply() accepted a null attachment list")
	}
	if reply, err := openAIFinalReply(`{"text":"","attachments":[]}`); err != nil || reply.Text != "" || len(reply.Attachments) != 0 {
		t.Fatalf("openAIFinalReply() empty reply = (%#v, %v)", reply, err)
	}
}

func TestOpenAIResponsesFinalReply(t *testing.T) {
	valid := openAIResponsesOutput{Type: "message", Status: "completed", Content: []openAIResponsesOutputContent{{Type: "output_text", Text: `{"text":"Done.","attachments":[]}`}}}
	reply, err := openAIResponsesFinalReply([]openAIResponsesOutput{{Type: "reasoning"}, valid})
	if err != nil || reply.Text != "Done." || len(reply.Attachments) != 0 {
		t.Fatalf("openAIResponsesFinalReply() = (%#v, %v)", reply, err)
	}
	later := openAIResponsesOutput{Type: "message", Status: "completed", Content: []openAIResponsesOutputContent{{Type: "output_text", Text: `{"text":"Later.","attachments":[]}`}}}
	reply, err = openAIResponsesFinalReply([]openAIResponsesOutput{valid, later})
	if err != nil || reply.Text != "Done." {
		t.Fatalf("openAIResponsesFinalReply() first valid output = (%#v, %v)", reply, err)
	}
	invalid := openAIResponsesOutput{Type: "message", Status: "completed", Content: []openAIResponsesOutputContent{{Type: "output_text", Text: `not JSON`}}}
	reply, err = openAIResponsesFinalReply([]openAIResponsesOutput{invalid, later})
	if err != nil || reply.Text != "Later." {
		t.Fatalf("openAIResponsesFinalReply() later valid output = (%#v, %v)", reply, err)
	}
	if _, err := openAIResponsesFinalReply([]openAIResponsesOutput{invalid}); err == nil {
		t.Fatal("openAIResponsesFinalReply() accepted no valid output text")
	}
}

func TestOpenAIRequestMessagesIncludesSharedSystemPrompt(t *testing.T) {
	emptyPrompt := ""
	messages := []openAICompatibleMessage{{Role: "user", Content: "hello"}}
	selected := &database.WorkspaceAgentModel{Alias: "luna-high"}
	prompt := openAISystemPromptFor(selected, false)
	withEmptyPrompt := openAIRequestMessages(&database.WorkspaceAgentModel{Alias: "luna-high", SystemPrompt: &emptyPrompt}, messages, false)
	if len(withEmptyPrompt) != 2 || withEmptyPrompt[0].Role != "system" || withEmptyPrompt[0].Content != prompt || withEmptyPrompt[1].Role != "user" || withEmptyPrompt[1].Content != "hello" {
		t.Fatalf("messages with empty prompt = %#v", withEmptyPrompt)
	}
	withDefaultPrompt := openAIRequestMessages(selected, messages, false)
	if len(withDefaultPrompt) != 2 || withDefaultPrompt[0].Role != "system" || withDefaultPrompt[0].Content != prompt || withDefaultPrompt[1].Role != "user" || withDefaultPrompt[1].Content != "hello" {
		t.Fatalf("messages with default prompt = %#v", withDefaultPrompt)
	}
	withProject := openAIRequestMessages(selected, messages, true)
	if len(withProject) != 2 || withProject[0].Content != openAISystemPromptFor(selected, true) {
		t.Fatalf("messages with project prompt = %#v", withProject)
	}
}

func TestAgentPreludeFor(t *testing.T) {
	custom := `(let ((custom #t)))`
	if got := agentPreludeFor(&database.WorkspaceAgentModel{}, false); got != agentSessionPrelude {
		t.Fatalf("ordinary chat prelude = %q", got)
	}
	if got := agentPreludeFor(&database.WorkspaceAgentModel{}, true); got != agentPrelude {
		t.Fatalf("project chat prelude = %q", got)
	}
	if got := agentPreludeFor(&database.WorkspaceAgentModel{Prelude: &custom}, false); got != custom {
		t.Fatalf("custom prelude = %q", got)
	}
}

func TestOpenAISystemPromptDocumentsIntegrationDiscovery(t *testing.T) {
	prompt := openAISystemPromptFor(&database.WorkspaceAgentModel{Alias: "luna-high"}, true)
	for _, binding := range []string{"session/*", "project/*", "help/env", "help/search"} {
		if !strings.Contains(prompt, binding) {
			t.Fatalf("system prompt does not document %q", binding)
		}
	}
}

func TestSessionPreludeDoesNotExposeProjectBindings(t *testing.T) {
	runtime := &SessionEventReplyRuntime{}
	session := model.SessionRef{}
	principal := model.PrincipalRef{}
	agent := model.WorkspaceAgentRef{}
	modules := []lisp.HostModule{
		runtime.sessionFilesModule(nil, session, principal),
		runtime.sessionNotesModule(nil, session, principal, agent),
		runtime.sessionTasksModule(nil, session, principal, agent),
		runtime.sessionSecretsModule(nil, session, principal),
		runtime.sessionEventsModule(nil, model.SessionEventRef{Session: session}, agent, principal),
		NewInputModule(), NewPolicyModule(nil), NewWebModule(),
	}
	err, result := lisp.Evaluate(`(and (> (list/length (help/env "session/")) 0) (= (list/length (help/env "project/")) 0))`, lisp.EvalOptions{Prelude: agentSessionPrelude, HostModules: modules})
	if err != nil || result == nil || result.String() != "#t" {
		t.Fatalf("session prelude exposed project bindings or lost session bindings: (%v, %v)", result, err)
	}
}

func TestSessionToolCallWorkflowIDUsesToolRequestID(t *testing.T) {
	request := model.SessionEventRef{Id: "sev_00000000000000000000000000"}
	if got, want := sessionToolCallWorkflowID(request), "session-tool-call:sev_00000000000000000000000000"; got != want {
		t.Fatalf("sessionToolCallWorkflowID() = %q, want %q", got, want)
	}
}

func TestSessionApprovalWorkflowIDUsesApprovalRequestID(t *testing.T) {
	request := model.SessionEventRef{Id: "sev_00000000000000000000000000"}
	if got, want := sessionApprovalWorkflowID(request), "session-approval:sev_00000000000000000000000000"; got != want {
		t.Fatalf("sessionApprovalWorkflowID() = %q, want %q", got, want)
	}
}

func TestSessionInputWorkflowIDUsesInputRequestID(t *testing.T) {
	request := model.SessionEventRef{Id: "sev_00000000000000000000000000"}
	if got, want := sessionInputWorkflowID(request), "session-input:sev_00000000000000000000000000"; got != want {
		t.Fatalf("sessionInputWorkflowID() = %q, want %q", got, want)
	}
}

func TestResourceModulesLoadInAgentPrelude(t *testing.T) {
	runtime := &SessionEventReplyRuntime{}
	session := model.SessionRef{}
	principal := model.PrincipalRef{}
	modules := []lisp.HostModule{
		runtime.projectInfoModule(nil, session, principal),
		runtime.projectFilesModule(nil, session, principal),
		runtime.projectNotesModule(nil, session, principal, model.WorkspaceAgentRef{}),
		runtime.projectTasksModule(nil, session, principal, model.WorkspaceAgentRef{}),
		runtime.projectSecretsModule(nil, session, principal),
		runtime.projectRecordsModule(nil, session, principal, model.WorkspaceAgentRef{}),
		runtime.sessionFilesModule(nil, session, principal),
		runtime.sessionNotesModule(nil, session, principal, model.WorkspaceAgentRef{}),
		runtime.sessionTasksModule(nil, session, principal, model.WorkspaceAgentRef{}),
		runtime.sessionSecretsModule(nil, session, principal),
		runtime.sessionEventsModule(nil, model.SessionEventRef{Session: session}, model.WorkspaceAgentRef{}, principal),
		NewInputModule(),
		NewPolicyModule(nil),
		NewWebModule(),
	}
	err, result := lisp.Evaluate(`null`, lisp.EvalOptions{Prelude: agentPrelude, HostModules: modules})
	if err != nil || result == nil {
		t.Fatalf("Evaluate() = (%v, %v)", result, err)
	}
}
