package agent

import (
	"fmt"
	"gatehouse/lisp"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

// ResultFileTestPublication exercises the real publication/transaction boundary
// with a transient outcome that can change between subprocess executions.
func ResultFileTestPublication(runtime *SessionEventReplyRuntime, outcome, body string) func(dbos.Context, model.SessionEventRef) (model.SessionEventRef, error) {
	return func(ctx dbos.Context, ref model.SessionEventRef) (model.SessionEventRef, error) {
		err, event := runtime.store.SessionEventGet(ctx, ref)
		if err != nil {
			return model.SessionEventRef{}, err
		}
		if event.Kind == model.SessionEventKindToolRequest {
			kind, code := model.SessionEventKindToolSuccess, ""
			if outcome == "failure" {
				kind, code = model.SessionEventKindToolFailure, "execution_failed"
			}
			return runtime.toolCallFinish(ctx, *event, *event.AuthorAgent, sessionToolCallExecution{Kind: kind, Code: code, Output: body})
		}
		err, selected := runtime.store.WorkspaceAgentModelGet(ctx, event.AuthorAgent.Workspace, event.AuthorAgent.Id)
		if err != nil {
			return model.SessionEventRef{}, err
		}
		turn := agentProviderTurn{Final: &agentFinalReply{Text: body}}
		if outcome == "failure" {
			turn = agentProviderTurn{Failure: fmt.Errorf("%s", body)}
		}
		if outcome == "retry" {
			turn = agentProviderTurn{Failure: &openAIRetryableError{}}
		}
		err, result := runtime.agentProviderDecisionCommit(ctx, *event.Parent, *event, selected, 0, 0, turn)
		return result, err
	}
}

// Test adapters let external integration tests migrate a real database without
// introducing an agent -> migrations -> agent import cycle in internal tests.
func ResultFileTestToolCall(runtime *SessionEventReplyRuntime) func(dbos.Context, model.SessionEventRef) (model.SessionEventRef, error) {
	return func(ctx dbos.Context, ref model.SessionEventRef) (model.SessionEventRef, error) {
		return runtime.toolCall(ctx, SessionToolCallInput{Request: ref})
	}
}

func ResultFileTestDiagnostic(runtime *SessionEventReplyRuntime, diagnostic string) func(dbos.Context, model.SessionEventRef) (model.SessionEventRef, error) {
	return func(ctx dbos.Context, ref model.SessionEventRef) (model.SessionEventRef, error) {
		err, event := runtime.store.SessionEventGet(ctx, ref)
		if err != nil {
			return model.SessionEventRef{}, err
		}
		err, selected := runtime.store.WorkspaceAgentModelGet(ctx, event.AuthorAgent.Workspace, event.AuthorAgent.Id)
		if err != nil {
			return model.SessionEventRef{}, err
		}
		err, outcome := runtime.agentProviderDecisionCommit(ctx, *event.Parent, *event, selected, 0, 0, agentProviderTurn{Failure: fmt.Errorf("%s", diagnostic)})
		return outcome, err
	}
}

func ResultFileTestTranscript(event model.SessionEvent, status string) string {
	return transcriptToolResultEvent(transcriptToolOutput{Event: event, Status: status}, "")
}

func ResultFileTestEvents(runtime *SessionEventReplyRuntime, ctx dbos.Context, request model.SessionEventRef, agent model.WorkspaceAgentRef, principal model.PrincipalRef) lisp.HostModule {
	return runtime.sessionEventsModule(ctx, request, agent, principal)
}
