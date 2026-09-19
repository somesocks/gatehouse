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

func TestOpenAISystemPromptFor(t *testing.T) {
	customPrompt := "Custom instructions."
	emptyPrompt := ""
	for name, prompt := range map[string]*string{
		"default": nil,
		"custom":  &customPrompt,
		"empty":   &emptyPrompt,
	} {
		t.Run(name, func(t *testing.T) {
			got := openAISystemPromptFor(&database.WorkspaceAgentModel{SystemPrompt: prompt})
			if name == "default" && got != openAISystemPrompt {
				t.Fatalf("system prompt = %q, want default prompt", got)
			}
			if name == "custom" && got != openAISystemPrompt+"\n\nCustom instructions." {
				t.Fatalf("system prompt = %q, want default prompt plus custom instructions", got)
			}
			if name == "empty" && got != openAISystemPrompt {
				t.Fatalf("system prompt = %q, want default prompt", got)
			}
		})
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

func TestOpenAIRequestMessagesIncludesDefaultSystemPrompt(t *testing.T) {
	emptyPrompt := ""
	messages := []openAICompatibleMessage{{Role: "user", Content: "hello"}}
	withEmptyPrompt := openAIRequestMessages(&database.WorkspaceAgentModel{SystemPrompt: &emptyPrompt}, messages)
	if len(withEmptyPrompt) != 2 || withEmptyPrompt[0].Role != "system" || withEmptyPrompt[0].Content != openAISystemPrompt || withEmptyPrompt[1].Role != "user" || withEmptyPrompt[1].Content != "hello" {
		t.Fatalf("messages with empty prompt = %#v", withEmptyPrompt)
	}
	withDefaultPrompt := openAIRequestMessages(&database.WorkspaceAgentModel{}, messages)
	if len(withDefaultPrompt) != 2 || withDefaultPrompt[0].Role != "system" || withDefaultPrompt[0].Content != openAISystemPrompt || withDefaultPrompt[1].Role != "user" || withDefaultPrompt[1].Content != "hello" {
		t.Fatalf("messages with default prompt = %#v", withDefaultPrompt)
	}
}

func TestAgentPreludeFor(t *testing.T) {
	custom := `(let ((custom #t)))`
	if got := agentPreludeFor(&database.WorkspaceAgentModel{}); got != agentPrelude {
		t.Fatalf("default prelude = %q", got)
	}
	if got := agentPreludeFor(&database.WorkspaceAgentModel{Prelude: &custom}); got != custom {
		t.Fatalf("custom prelude = %q", got)
	}
}

func TestOpenAISystemPromptDocumentsIntegrationDiscovery(t *testing.T) {
	for _, binding := range []string{"session/*", "project/*", "help/env", "help/search"} {
		if !strings.Contains(openAISystemPrompt, binding) {
			t.Fatalf("system prompt does not document %q", binding)
		}
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
		runtime.sessionEventsModule(nil, session, principal),
		NewPolicyModule(nil),
		NewWebModule(),
	}
	err, result := lisp.Evaluate(`null`, lisp.EvalOptions{Prelude: agentPrelude, HostModules: modules})
	if err != nil || result == nil {
		t.Fatalf("Evaluate() = (%v, %v)", result, err)
	}
}
