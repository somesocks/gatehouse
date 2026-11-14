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

// Ordinary worker memory only, never a workflow/step input or result. Agent is
// the existing selected workspace agent, not a new identity/profile resource.
type tadwPreparedHistory struct {
	Agent   model.WorkspaceAgentRef
	Scope   tadwHistoryCacheScope
	History tadwInitialHistory
}

// prepareTADWHistory belongs once at request entry. The selected agent must
// already contain any invocation-local prompt replacement. No active workflow
// calls it. It neither sends a provider request nor publishes/touches a hint.
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
	if selected == nil || selected.Ref.Workspace != session.Workspace || !typed_id.Valid(typed_id.WorkspaceAgent, selected.Ref.Id) || !typed_id.Valid(typed_id.AgentModel, selected.AgentModel.Id) || !typed_id.Valid(typed_id.AgentProvider, selected.ProviderID) || strings.TrimSpace(selected.Model) == "" {
		return tadwHistoryCacheScope{}, fmt.Errorf("prepare TADW history: invalid selected workspace agent")
	}
	if selected.Protocol == "builtin" {
		return tadwHistoryCacheScope{}, fmt.Errorf("prepare TADW history: builtin agents have no provider cache scope")
	}
	if selected.Endpoint == nil || strings.TrimSpace(*selected.Endpoint) == "" || selected.Keychain == nil || selected.Keychain.Id == "" || selected.Keychain.Version <= 0 || selected.APIKey == nil || strings.TrimSpace(*selected.APIKey) == "" {
		return tadwHistoryCacheScope{}, fmt.Errorf("prepare TADW history: incomplete provider configuration")
	}
	if selected.Protocol == config.AgentProviderProtocolOpenAICodex && *selected.Endpoint != config.OpenAICodexEndpoint {
		return tadwHistoryCacheScope{}, fmt.Errorf("prepare TADW history: Codex requires its fixed endpoint")
	}
	err, template := agentProviderRequestPrepare(selected, projectChat, nil)
	if err != nil {
		return tadwHistoryCacheScope{}, err
	}
	// Reuse the existing opaque credential-bundle fingerprint. Do not decrypt,
	// persist ciphertext, or add credential metadata reads. Account/key changes,
	// refresh and re-encryption conservatively separate scopes.
	credential := database.AgentProviderCredential{Provider: database.SystemAgentProvider{Protocol: selected.Protocol, Keychain: selected.Keychain}, Encrypted: selected.APIKey}
	cacheScope, err := tadwPreparationDigest(struct {
		Version                                                    string
		Agent                                                      model.WorkspaceAgentRef
		AgentModel                                                 model.AgentModelRef
		Provider, Protocol, Endpoint, Model, CredentialFingerprint string
	}{tadwHistoryPreparationVersion, selected.Ref, selected.AgentModel, selected.ProviderID, selected.Protocol, *selected.Endpoint, selected.Model, credential.Fingerprint()})
	if err != nil {
		return tadwHistoryCacheScope{}, err
	}
	profile, err := tadwPreparationProfileFingerprint(template)
	if err != nil {
		return tadwHistoryCacheScope{}, err
	}
	scope := tadwHistoryCacheScope{Session: session, CacheScope: cacheScope, Profile: profile}
	if err := scope.databaseScope().Validate(); err != nil {
		return tadwHistoryCacheScope{}, err
	}
	return scope, nil
}

// template must come from agentProviderRequestPrepare with nil conversation.
// body() includes the real Codex wire transformation, rather than hashing the
// intermediate Responses request (which has settings Codex does not send).
func tadwPreparationProfileFingerprint(template agentProviderRequest) (string, error) {
	return tadwPreparationDigest(struct {
		Version, Algorithm, TranscriptFormat string
		StaticRequest                        any
	}{tadwHistoryPreparationVersion, tadwHistoryCacheAlgorithm, tadwTranscriptFormatVersion, template.body()})
}

func tadwPreparationDigest(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode TADW history compatibility: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
