package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

const tadwHistoryCacheAlgorithm = "tadw-history-v1"

type tadwHistoryCacheScope struct {
	Session    model.SessionRef
	CacheScope string
	Profile    string
}

func (scope tadwHistoryCacheScope) databaseScope() database.ContextCacheHintScope {
	return database.ContextCacheHintScope{Session: scope.Session, CacheScope: scope.CacheScope, Algorithm: tadwHistoryCacheAlgorithm, Profile: scope.Profile}
}

type tadwHistoryCacheHint struct {
	Version     int      `json:"version"`
	BaseEventID string   `json:"base_event_id"`
	Breakpoints []string `json:"breakpoints,omitempty"` // Marked event IDs in sent order.
}

func parseTADWHistoryCacheHint(record database.ContextCacheHint) (tadwHistoryCacheHint, bool) {
	var hint tadwHistoryCacheHint
	if len(record.Hint) > database.ContextCacheHintMaxBytes || json.Unmarshal(record.Hint, &hint) != nil || hint.Version != 1 || !typed_id.Valid(typed_id.SessionEvent, hint.BaseEventID) || record.Key != hint.BaseEventID {
		return tadwHistoryCacheHint{}, false
	}
	return hint, true
}

// Hints never imply membership/visibility or a provider cache hit. The existing
// selector intersects these IDs with its bounded candidate and ignores misses.
func loadTADWHistoryCacheHints(ctx context.Context, store *database.Store, scope tadwHistoryCacheScope) ([]string, error) {
	hints, err := store.ContextCacheHintsList(ctx, scope.databaseScope())
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, record := range hints {
		if hint, ok := parseTADWHistoryCacheHint(record); ok {
			ids = append(ids, hint.BaseEventID)
		}
	}
	return ids, nil
}

// Breakpoint candidates from another base cannot match the full prefix. Only
// event IDs are shared: the provider still verifies exact equality and TTL.
func loadTADWCacheBreakpoints(ctx context.Context, store *database.Store, scope tadwHistoryCacheScope, base string) ([]string, error) {
	records, err := store.ContextCacheHintsList(ctx, scope.databaseScope())
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		if record.Key != base {
			continue
		}
		hint, ok := parseTADWHistoryCacheHint(record)
		if ok && len(hint.Breakpoints) <= tadwMaxProviderBreakpoints {
			return hint.Breakpoints, nil
		}
		break
	}
	return nil, nil
}

// publishTADWHistoryCacheHint must be explicitly called only after a provider
// request actually used this base. Merely selecting/checkpointing never calls it.
// IDs can become stale; no event payload read or durable-context write is needed.
func publishTADWHistoryCacheHint(ctx context.Context, store *database.Store, scope tadwHistoryCacheScope, base model.SessionEventRef, breakpoints ...string) error {
	if base.Session != scope.Session || !typed_id.Valid(typed_id.SessionEvent, base.Id) {
		return fmt.Errorf("publish TADW history cache hint: invalid base scope or ID")
	}
	if len(breakpoints) > tadwMaxProviderBreakpoints {
		return fmt.Errorf("publish TADW history cache hint: too many breakpoints")
	}
	hint, err := json.Marshal(tadwHistoryCacheHint{Version: 1, BaseEventID: base.Id, Breakpoints: breakpoints})
	if err != nil {
		return err
	}
	return store.ContextCacheHintPut(ctx, scope.databaseScope(), base.Id, hint)
}

// Derive publication identity from the validated selected snapshot and the wire
// request used by this attempt. Only the conversation is removed; actual static
// fields, including output caps, remain. Neither events nor configuration are read.
func tadwHistoryCacheScopeForSentRequest(session model.SessionRef, selected *database.WorkspaceAgentModel, request agentProviderRequest) (tadwHistoryCacheScope, error) {
	if selected == nil || selected.Endpoint == nil || selected.Ref.Workspace != session.Workspace {
		return tadwHistoryCacheScope{}, fmt.Errorf("invalid TADW publication provider snapshot")
	}
	template := agentProviderRequest{codex: request.codex}
	switch {
	case request.Chat != nil && request.Responses == nil && !request.codex:
		chat := *request.Chat
		systemCount := 0
		if len(chat.Messages) > 0 && chat.Messages[0].Role == "system" {
			systemCount = 1
		}
		users := chat.Messages[systemCount:]
		if len(users) < 2 { // At least the owning request and the separate hints message.
			return tadwHistoryCacheScope{}, fmt.Errorf("invalid TADW publication conversation")
		}
		for _, message := range users {
			if message.Role != "user" {
				return tadwHistoryCacheScope{}, fmt.Errorf("invalid TADW publication conversation")
			}
		}
		if systemCount == 0 {
			chat.Messages = nil
		} else {
			chat.Messages = chat.Messages[:1:1]
		}
		template.Chat = &chat
	case request.Responses != nil && request.Chat == nil && len(request.Responses.Input) >= 2:
		responses := *request.Responses
		responses.Input = []json.RawMessage{}
		template.Responses = &responses
	default:
		return tadwHistoryCacheScope{}, fmt.Errorf("invalid TADW publication request")
	}
	cacheScope, err := tadwPreparationCacheScope(selected)
	if err != nil {
		return tadwHistoryCacheScope{}, err
	}
	profile, err := tadwPreparationProfileFingerprint(template)
	if err != nil {
		return tadwHistoryCacheScope{}, err
	}
	scope := tadwHistoryCacheScope{Session: session, CacheScope: cacheScope, Profile: profile}
	return scope, scope.databaseScope().Validate()
}

// checkpointTADWHistoryWithCacheHints keeps exactly the original ID-only step.
// Hint lookup is fresh-only, before opening the history read transaction, so
// replay does not depend on the hints table and the sole connection is not nested.
func checkpointTADWHistoryWithCacheHints(ctx dbos.Context, store *database.Store, anchor model.SessionEventRef, scope tadwHistoryCacheScope, config tadwWindowConfig) (tadwInitialHistory, error) {
	if anchor.Session != scope.Session {
		return tadwInitialHistory{}, fmt.Errorf("checkpoint TADW history: mismatched cache scope")
	}
	if err := scope.databaseScope().Validate(); err != nil {
		return tadwInitialHistory{}, err
	}
	return checkpointTADWHistorySelection(ctx, store, anchor, config, func(stepCtx context.Context) (tadwInitialHistory, error) {
		ids, err := loadTADWHistoryCacheHints(stepCtx, store, scope)
		if err != nil {
			return tadwInitialHistory{}, err
		}
		return loadTADWHistory(stepCtx, store, anchor, ids, config)
	})
}
