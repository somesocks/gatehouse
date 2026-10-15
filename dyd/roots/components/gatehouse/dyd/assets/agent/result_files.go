package agent

import (
	"context"
	"fmt"
	"strings"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

type agentResultBody struct {
	Output     *string
	OutputFile *model.SessionFileRef
	Message    *string
}

// Result contents are published before the terminal event transaction. A crash
// between publication and commit may leave an unreferenced file for later GC.
func (runtime *SessionEventReplyRuntime) agentResultStore(ctx context.Context, source model.SessionEvent, body string, cutoff int) (agentResultBody, error) {
	if cutoff == 0 {
		cutoff = config.DefaultAgentModelResultInlineBytes
	}
	if len(body) <= cutoff {
		return agentResultBody{Output: &body}, nil
	}
	principal, err := runtime.resultPrincipal(ctx, source)
	if err != nil {
		return agentResultBody{}, err
	}
	err, file, object := runtime.store.SessionResultFileCreate(ctx, source.Ref, principal)
	if err != nil {
		return agentResultBody{}, fmt.Errorf("create result artifact: %w", err)
	}
	if file == nil || object == nil {
		return agentResultBody{}, fmt.Errorf("create result artifact: unavailable")
	}
	if err := runtime.storage.Put(ctx, object.ID, strings.NewReader(body), int64(len(body))); err != nil {
		return agentResultBody{}, fmt.Errorf("store result artifact: %w", err)
	}
	if err := runtime.storage.Finish(ctx, object.ID); err != nil {
		return agentResultBody{}, fmt.Errorf("finish result artifact: %w", err)
	}
	return agentResultBody{OutputFile: &file.Ref, Message: resultFailureSummary(body)}, nil
}

func (runtime *SessionEventReplyRuntime) resultPrincipal(ctx context.Context, source model.SessionEvent) (model.PrincipalRef, error) {
	if source.Parent == nil {
		return model.PrincipalRef{}, fmt.Errorf("result request has no parent")
	}
	err, parent := runtime.store.SessionEventGet(ctx, *source.Parent)
	if err != nil {
		return model.PrincipalRef{}, err
	}
	if parent == nil || parent.AuthorPrincipal == nil {
		return model.PrincipalRef{}, fmt.Errorf("result request principal is unavailable")
	}
	return parent.AuthorPrincipal.Ref, nil
}

func resultInlineLimit(selected *database.WorkspaceAgentModel) int {
	if selected == nil || selected.ResultInlineBytes == 0 {
		return config.DefaultAgentModelResultInlineBytes
	}
	return selected.ResultInlineBytes
}

// File-backed failures keep a bounded first-line summary rather than a preview
// of a potentially enormous provider response or Lisp diagnostic.
func resultFailureSummary(body string) *string {
	line, _, _ := strings.Cut(body, "\n")
	line = mcmtrLimitText(line, 256).value
	return optionalString(line)
}
