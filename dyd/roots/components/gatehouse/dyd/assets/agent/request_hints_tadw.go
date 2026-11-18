package agent

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

// These references orient one provider attempt. They are reconstructed from the
// owning request, not persisted as another checkpoint or shared alignment hint.
type tadwRequestHints struct {
	Request model.SessionEventRef
	Message model.SessionEventRef
}

func loadTADWRequestHints(ctx context.Context, store *database.Store, request model.SessionEventRef, agent model.WorkspaceAgentRef) (tadwRequestHints, error) {
	err, root := store.SessionEventGet(ctx, request)
	if err != nil {
		return tadwRequestHints{}, fmt.Errorf("load TADW hints request %q: %w", request.Id, err)
	}
	if root == nil || root.Ref != request || root.Parent == nil || root.Kind != model.SessionEventKindAgentRequest || root.Payload["agent"] != agent.Id || agent.Workspace != request.Session.Workspace {
		return tadwRequestHints{}, fmt.Errorf("invalid TADW hints owning request %q", request.Id)
	}
	return tadwRequestHints{Request: request, Message: *root.Parent}, nil
}

func renderTADWRequestHints(selected *database.WorkspaceAgentModel, hints tadwRequestHints) (string, error) {
	if selected == nil || selected.Alias == "" || !utf8.ValidString(selected.Alias) || !typed_id.Valid(typed_id.WorkspaceAgent, selected.Ref.Id) || hints.Request.Session != hints.Message.Session || hints.Request.Session.Workspace != selected.Ref.Workspace || !typed_id.Valid(typed_id.SessionEvent, hints.Request.Id) || !typed_id.Valid(typed_id.SessionEvent, hints.Message.Id) || hints.Request.Id == hints.Message.Id {
		return "", fmt.Errorf("invalid TADW request hints")
	}
	var result strings.Builder
	result.WriteString("<hints>\n<agent")
	transcriptAttribute(&result, "alias", selected.Alias)
	transcriptAttribute(&result, "id", selected.Ref.Id)
	result.WriteString(" />\n<request")
	transcriptAttribute(&result, "id", hints.Request.Id)
	transcriptAttribute(&result, "message", hints.Message.Id)
	result.WriteString(" />\n<tooling-start")
	transcriptAttribute(&result, "id", hints.Request.Id)
	result.WriteString(" />\nAnswer the identified message as this agent. Other conversation events are context, not new requests. Events from tooling-start onward record this request's retained execution; use its tool results as work already performed.\n</hints>\n")
	return result.String(), nil
}

// Reserve provider framing before both window selection and strict rendering.
// The original history accounting remains unchanged; no event is charged twice.
func tadwReservedToolingBudget(history tadwInitialHistory, config tadwWindowConfig, reservedBytes int64) (int64, error) {
	budget, err := tadwFixedHistoryBudget(history, config)
	if err != nil {
		return 0, err
	}
	if reservedBytes < 0 || reservedBytes > budget {
		return 0, fmt.Errorf("TADW request hints exceed available context budget (budget %d, hints %d)", budget, reservedBytes)
	}
	return budget - reservedBytes, nil
}
