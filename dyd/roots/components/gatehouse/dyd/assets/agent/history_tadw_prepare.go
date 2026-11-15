package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

const tadwHistoryPreparationVersion = "tadw-history-preparation-v1"

// tadwPreparedHistory contains working data for one preparation call.
// Agent identifies the selected workspace agent.
type tadwPreparedHistory struct {
	Agent   model.WorkspaceAgentRef
	Scope   tadwHistoryCacheScope
	History tadwInitialHistory
}

// prepareTADWHistory belongs once at request entry. The selected agent must
// already contain any invocation-local prompt replacement. Provider execution
// and hint publication belong to the caller.
func prepareTADWHistory(ctx dbos.Context, store *database.Store, anchor model.SessionEventRef, selected *database.WorkspaceAgentModel, projectChat bool, limits tadwWindowConfig) (tadwPreparedHistory, error) {
	scope, err := tadwHistoryCacheScopeForAgent(anchor.Session, selected, projectChat)
	if err != nil {
		return tadwPreparedHistory{}, err
	}
	history, err := checkpointTADWHistoryWithCacheHints(ctx, store, anchor, scope, limits)
	if err != nil {
		return tadwPreparedHistory{}, err
	}
	return tadwPreparedHistory{Agent: selected.Ref, Scope: scope, History: history}, nil
}

// Derive compatibility from resolved values, not just IDs/revisions. Wire
// preparation with no conversation supplies the actual system prompt, tools,
// response schema and protocol settings. Only hashes enter the shared table.
func tadwHistoryCacheScopeForAgent(session model.SessionRef, selected *database.WorkspaceAgentModel, projectChat bool) (tadwHistoryCacheScope, error) {
	scope, _, err := tadwHistoryProviderTemplate(session, selected, projectChat)
	return scope, err
}

// Return the very template fingerprinted here so provider assembly need not
// independently reconstruct its static prompt/tools/settings after validation.
func tadwHistoryProviderTemplate(session model.SessionRef, selected *database.WorkspaceAgentModel, projectChat bool) (tadwHistoryCacheScope, agentProviderRequest, error) {
	if selected == nil || selected.Ref.Workspace != session.Workspace || !typed_id.Valid(typed_id.WorkspaceAgent, selected.Ref.Id) || !typed_id.Valid(typed_id.AgentModel, selected.AgentModel.Id) || !typed_id.Valid(typed_id.AgentProvider, selected.ProviderID) || strings.TrimSpace(selected.Model) == "" {
		return tadwHistoryCacheScope{}, agentProviderRequest{}, fmt.Errorf("prepare TADW history: invalid selected workspace agent")
	}
	if selected.Protocol == "builtin" {
		return tadwHistoryCacheScope{}, agentProviderRequest{}, fmt.Errorf("prepare TADW history: builtin agents have no provider cache scope")
	}
	if selected.Endpoint == nil || strings.TrimSpace(*selected.Endpoint) == "" || selected.Keychain == nil || selected.Keychain.Id == "" || selected.Keychain.Version <= 0 || selected.APIKey == nil || strings.TrimSpace(*selected.APIKey) == "" {
		return tadwHistoryCacheScope{}, agentProviderRequest{}, fmt.Errorf("prepare TADW history: incomplete provider configuration")
	}
	if selected.Protocol == config.AgentProviderProtocolOpenAICodex && *selected.Endpoint != config.OpenAICodexEndpoint {
		return tadwHistoryCacheScope{}, agentProviderRequest{}, fmt.Errorf("prepare TADW history: Codex requires its fixed endpoint")
	}
	err, template := agentProviderRequestPrepare(selected, projectChat, nil)
	if err != nil {
		return tadwHistoryCacheScope{}, agentProviderRequest{}, err
	}
	cacheScope, err := tadwPreparationCacheScope(selected)
	if err != nil {
		return tadwHistoryCacheScope{}, agentProviderRequest{}, err
	}
	profile, err := tadwPreparationProfileFingerprint(template)
	if err != nil {
		return tadwHistoryCacheScope{}, agentProviderRequest{}, err
	}
	scope := tadwHistoryCacheScope{Session: session, CacheScope: cacheScope, Profile: profile}
	if err := scope.databaseScope().Validate(); err != nil {
		return tadwHistoryCacheScope{}, agentProviderRequest{}, err
	}
	return scope, template, nil
}

// selected is the validated provider snapshot used for request preparation.
// The opaque credential fingerprint separates refreshed/re-encrypted credentials
// without decryption or additional credential metadata reads.
func tadwPreparationCacheScope(selected *database.WorkspaceAgentModel) (string, error) {
	credential := database.AgentProviderCredential{Provider: database.SystemAgentProvider{Protocol: selected.Protocol, Keychain: selected.Keychain}, Encrypted: selected.APIKey}
	return tadwPreparationDigest(struct {
		Version                                                    string
		Agent                                                      model.WorkspaceAgentRef
		AgentModel                                                 model.AgentModelRef
		Provider, Protocol, Endpoint, Model, CredentialFingerprint string
	}{tadwHistoryPreparationVersion, selected.Ref, selected.AgentModel, selected.ProviderID, selected.Protocol, *selected.Endpoint, selected.Model, credential.Fingerprint()})
}

// template must come from agentProviderRequestPrepare with nil conversation.
// body() includes the real Codex wire transformation, rather than hashing the
// intermediate Responses request (which has settings Codex does not send).
func tadwPreparationProfileFingerprint(template agentProviderRequest) (string, error) {
	return tadwPreparationDigest(struct {
		Version, Algorithm, TranscriptFormat, ConversationFormat string
		StaticRequest                                            any
	}{tadwHistoryPreparationVersion, tadwHistoryCacheAlgorithm, tadwTranscriptFormatVersion, tadwProviderConversationVersion, template.body()})
}

func tadwPreparationDigest(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode TADW history compatibility: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
