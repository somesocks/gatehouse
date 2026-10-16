package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"gatehouse/model"
	"gatehouse/typed_id"
)

type AgentProviderCredential struct {
	Provider  SystemAgentProvider
	Encrypted *string `json:"-"`
}

// Fingerprint is an opaque reference derived from the existing ciphertext and
// encryption context. It can be checkpointed without copying credential data;
// it is never stored as a provider field or inside the credential bundle.
func (record AgentProviderCredential) Fingerprint() string {
	return agentProviderCredentialFingerprint(record.Provider.Protocol, record.Provider.Keychain, record.Encrypted)
}

func agentProviderCredentialFingerprint(protocol string, ref *model.KeychainRef, encrypted *string) string {
	encoded, _ := json.Marshal(struct {
		Protocol   string
		Keychain   *model.KeychainRef
		Credential *string
	}{protocol, ref, encrypted})
	defer clear(encoded)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

// Read the configuration and ciphertext in one snapshot. Never expose this
// internal record directly in an administration response or workflow result.
func (store *Store) AgentProviderCredentialGet(ctx context.Context, id string) (error, *AgentProviderCredential) {
	if !typed_id.Valid(typed_id.AgentProvider, id) {
		return nil, nil
	}
	var encrypted sql.NullString
	p := keychainPlaceholder(store.kind)
	provider, err := scanSystemAgentProvider(store.QueryRowContext(ctx, `
		SELECT id, alias, revision, protocol, endpoint, keychain_id, keychain_version, credential IS NOT NULL, enabled, credential_metadata, credential
		FROM gatehouse_agent_providers WHERE id = `+p(1), id), &encrypted)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("read provider credential: %w", err), nil
	}
	record := &AgentProviderCredential{Provider: provider}
	if encrypted.Valid {
		record.Encrypted = &encrypted.String
	}
	return nil, record
}

// Manual changes use the configuration revision. Runtime maintenance compares
// the original ciphertext, so credential changes also fence in-flight refreshes.
func (store *Store) SystemAgentProviderCodexCredentialSet(ctx context.Context, id string, revision int, ref *model.KeychainRef, encrypted *string, account *SystemAgentProviderAccount) (error, *SystemAgentProvider) {
	if !typed_id.Valid(typed_id.AgentProvider, id) || revision < 1 {
		return fmt.Errorf("invalid provider credential update"), nil
	}
	var metadata any
	if encrypted != nil {
		if strings.TrimSpace(*encrypted) == "" || ref == nil || account == nil || strings.TrimSpace(account.ID) == "" {
			return fmt.Errorf("invalid Codex credential"), nil
		}
		encoded, err := json.Marshal(account)
		if err != nil {
			return fmt.Errorf("encode provider account metadata"), nil
		}
		metadata = string(encoded)
	} else if account != nil || ref != nil {
		return fmt.Errorf("invalid provider disconnect"), nil
	}
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return err, nil
	}
	defer tx.Rollback()
	p := keychainPlaceholder(store.kind)
	// Write first so SQLite serializes competing completions before either
	// transaction establishes a read snapshot. The revision CAS chooses a winner.
	result, err := tx.ExecContext(ctx, `UPDATE gatehouse_agent_providers SET
		credential = `+p(1)+`, credential_metadata = `+p(2)+`, revision = revision + 1,
		keychain_id = COALESCE(`+p(3)+`, keychain_id), keychain_version = COALESCE(`+p(4)+`, keychain_version)
		WHERE id = `+p(5)+` AND revision = `+p(6)+` AND protocol = 'openai-codex'`, optionalStringValue(encrypted), metadata, keychainID(ref), keychainVersion(ref), id, revision)
	if err != nil {
		return fmt.Errorf("update provider credential: %w", err), nil
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err, nil
	}
	if count != 1 {
		var exists string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM gatehouse_agent_providers WHERE id = `+p(1), id).Scan(&exists); err == sql.ErrNoRows {
			return ErrAdminDependencyMissing, nil
		} else if err != nil {
			return err, nil
		}
		return ErrRevisionConflict, nil
	}
	if err := store.systemKeychainExists(ctx, tx, ref); err != nil {
		return err, nil
	}
	if err, _ := store.ActivityEventAppend(ctx, tx, model.ActivityEvent{Event: "agent_provider.update", ResourceKind: ActivityResourceKindAgentProvider, ResourceAgentProvider: &id}, []string{"sys/" + id}); err != nil {
		return err, nil
	}
	provider, err := scanSystemAgentProvider(tx.QueryRowContext(ctx, `SELECT id, alias, revision, protocol, endpoint, keychain_id, keychain_version, credential IS NOT NULL, enabled, credential_metadata FROM gatehouse_agent_providers WHERE id = `+p(1), id))
	if err != nil {
		return err, nil
	}
	if err := tx.Commit(); err != nil {
		return err, nil
	}
	return nil, &provider
}
