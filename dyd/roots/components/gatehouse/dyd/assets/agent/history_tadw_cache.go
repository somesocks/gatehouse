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
	Version     int    `json:"version"`
	BaseEventID string `json:"base_event_id"`
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
		var hint tadwHistoryCacheHint
		if len(record.Hint) > database.ContextCacheHintMaxBytes || json.Unmarshal(record.Hint, &hint) != nil || hint.Version != 1 || !typed_id.Valid(typed_id.SessionEvent, hint.BaseEventID) || record.Key != hint.BaseEventID {
			continue
		}
		ids = append(ids, hint.BaseEventID)
	}
	return ids, nil
}

// publishTADWHistoryCacheHint must be explicitly called only after a provider
// request actually used this base. Merely selecting/checkpointing never calls it.
// IDs can become stale; no event payload read or durable-context write is needed.
func publishTADWHistoryCacheHint(ctx context.Context, store *database.Store, scope tadwHistoryCacheScope, base model.SessionEventRef) error {
	if base.Session != scope.Session || !typed_id.Valid(typed_id.SessionEvent, base.Id) {
		return fmt.Errorf("publish TADW history cache hint: invalid base scope or ID")
	}
	hint, err := json.Marshal(tadwHistoryCacheHint{Version: 1, BaseEventID: base.Id})
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
		switch {
		case len(chat.Messages) == 1 && chat.Messages[0].Role == "user":
			chat.Messages = nil
		case len(chat.Messages) == 2 && chat.Messages[0].Role == "system" && chat.Messages[1].Role == "user":
			chat.Messages = chat.Messages[:1:1]
		default:
			return tadwHistoryCacheScope{}, fmt.Errorf("invalid TADW publication conversation")
		}
		template.Chat = &chat
	case request.Responses != nil && request.Chat == nil && len(request.Responses.Input) == 1:
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
