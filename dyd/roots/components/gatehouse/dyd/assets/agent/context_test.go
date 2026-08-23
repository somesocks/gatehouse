package agent

import (
	"testing"

	"gatehouse/model"
)

func TestSessionContextMessagesKeepsEventsAfterCheckpointBoundary(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	principal := model.Principal{Ref: model.PrincipalRef{Id: "user"}, Enabled: true}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Model: model.AgentModelRef{Id: "amd_01arz3ndektsv4rrffq69g5fav"}}
	older := event(session, "older", "message.text", &principal, nil, map[string]interface{}{"text": "Older request."})
	boundary := event(session, "boundary", "message.text", nil, &agent, map[string]interface{}{"text": "Older reply."})
	started := event(session, "started", "context.compaction.started", nil, &agent, map[string]interface{}{})
	started.Parent = &older.Ref
	completed := event(session, "completed", "context.compaction.completed", nil, &agent, map[string]interface{}{"summary": "Older work is complete."})
	completed.Parent = &started.Ref
	newer := event(session, "newer", "message.text", &principal, nil, map[string]interface{}{"text": "New request."})

	checkpoint, messages, err := sessionContextMessages([]model.SessionEvent{older, boundary, started, completed, newer})
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint == nil || checkpoint.Index != 2 || checkpoint.Summary != "Older work is complete." {
		t.Fatalf("checkpoint = %#v", checkpoint)
	}
	if len(messages) != 2 || messages[0].Role != "user" || messages[0].Content != "Session summary of earlier history:\nOlder work is complete." || messages[1].Role != "user" || messages[1].Content != "New request." {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestLatestContextCheckpointRejectsInvalidParentage(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "workspace"}, Id: "session"}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Model: model.AgentModelRef{Id: "amd_01arz3ndektsv4rrffq69g5fav"}}
	completed := event(session, "completed", "context.compaction.completed", nil, &agent, map[string]interface{}{"summary": "invalid"})
	completed.Parent = &model.SessionEventRef{Session: session, Id: "missing"}
	if checkpoint := latestContextCheckpoint([]model.SessionEvent{completed}); checkpoint != nil {
		t.Fatalf("checkpoint = %#v, want nil", checkpoint)
	}
}

func TestPreviewToolOutputsKeepsLedgerSizedContext(t *testing.T) {
	messages := previewToolOutputs([]openAICompatibleMessage{{Role: "tool", Content: "abcdefghij"}}, 2)
	if len(messages) != 1 || messages[0].Content != "abcdef\n\n[Tool output truncated for the context budget.]" {
		t.Fatalf("messages = %#v", messages)
	}
}
