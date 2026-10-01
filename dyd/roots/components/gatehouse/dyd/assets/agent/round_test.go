package agent

import (
	"encoding/json"
	"testing"

	"gatehouse/database"
	"gatehouse/model"
)

func TestAgentRoundReferenceInterfaceAndIdentity(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "wsp_00000000000000000000000000"}, Id: "ses_00000000000000000000000000"}
	root := model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}
	previous := model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"}
	first := SessionAgentRoundInput{AgentRequest: root}
	next := SessionAgentRoundInput{AgentRequest: root, Previous: &previous}
	if sessionAgentRoundWorkflowID(first) != "session-agent-round:"+root.Id+":first" || sessionAgentRoundWorkflowID(next) != "session-agent-round:"+root.Id+":after:"+previous.Id {
		t.Fatal("round identity must be stable and predecessor-bound")
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields["AgentRequest"] == nil || fields["Previous"] == nil {
		t.Fatalf("round input must contain only existing references: %s", encoded)
	}
}

func TestAgentRoundDecisionsRequireOneValidVariant(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "wsp_00000000000000000000000000"}, Id: "ses_00000000000000000000000000"}
	ref := model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"}
	tools := []model.SessionEventRef{ref}
	emptyTools := []model.SessionEventRef{}
	duplicates := []model.SessionEventRef{ref, ref}
	other := ref
	other.Session.Id = "ses_00000000000000000000000002"
	for _, test := range []struct {
		name  string
		value model.ThinkingSuccessPayload
		valid bool
	}{
		{"tools", model.ThinkingSuccessPayload{ToolRequests: &tools}, true},
		{"final", model.ThinkingSuccessPayload{AgentSuccess: &ref}, true},
		{"empty", model.ThinkingSuccessPayload{}, false},
		{"empty tools", model.ThinkingSuccessPayload{ToolRequests: &emptyTools}, false},
		{"both variants", model.ThinkingSuccessPayload{ToolRequests: &tools, AgentSuccess: &ref}, false},
		{"duplicate tools", model.ThinkingSuccessPayload{ToolRequests: &duplicates}, false},
		{"another session", model.ThinkingSuccessPayload{AgentSuccess: &other}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload, err := database.SessionEventPayloadFrom(test.value)
			if err != nil {
				t.Fatal(err)
			}
			_, err = agentRoundDecision(&model.SessionEvent{Ref: ref, Kind: model.SessionEventKindThinkingSuccess, Payload: payload})
			if (err == nil) != test.valid {
				t.Fatalf("decision validity = %v, want %v: %v", err == nil, test.valid, err)
			}
		})
	}
}

func TestAgentRoundHistoryFollowsExplicitPredecessors(t *testing.T) {
	session := model.SessionRef{Workspace: model.WorkspaceRef{Id: "wsp_00000000000000000000000000"}, Id: "ses_00000000000000000000000000"}
	agent := model.WorkspaceAgentRef{Workspace: session.Workspace, Id: "wag_00000000000000000000000000"}
	root := model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000000"}
	thinking := model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000001"}
	tool := model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000002"}
	outcome := model.SessionEventRef{Session: session, Id: "sev_00000000000000000000000003"}
	tools := []model.SessionEventRef{tool}
	payload, err := database.SessionEventPayloadFrom(model.ThinkingSuccessPayload{ToolRequests: &tools})
	if err != nil {
		t.Fatal(err)
	}
	events := map[string]*model.SessionEvent{
		thinking.Id: {Ref: thinking, Parent: &root, Kind: model.SessionEventKindThinkingRequest, AuthorAgent: &agent, Payload: map[string]interface{}{"turn": 0}},
		tool.Id:     {Ref: tool, Parent: &root, Kind: model.SessionEventKindToolRequest, AuthorAgent: &agent},
		outcome.Id:  {Ref: outcome, Parent: &thinking, Kind: model.SessionEventKindThinkingSuccess, AuthorAgent: &agent, Payload: payload},
	}
	get := func(ref model.SessionEventRef) (error, *model.SessionEvent) { return nil, events[ref.Id] }
	input := SessionAgentRoundInput{AgentRequest: root, Previous: &outcome}
	round, offset, err := agentRoundHistory(input, agent, get)
	if err != nil || round != 1 || offset != 1 {
		t.Fatalf("predecessor history = (%d, %d, %v)", round, offset, err)
	}
	otherRoot := root
	otherRoot.Id = "sev_00000000000000000000000004"
	events[thinking.Id].Parent = &otherRoot
	if _, _, err := agentRoundHistory(input, agent, get); err == nil {
		t.Fatal("accepted a predecessor from another root request")
	}
	events[thinking.Id].Parent = &root
	events[thinking.Id].Payload["previous"] = outcome
	if _, _, err := agentRoundHistory(input, agent, get); err == nil {
		t.Fatal("accepted a cyclic predecessor chain")
	}
}
