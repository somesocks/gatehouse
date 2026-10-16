package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"gatehouse/model"
	"gatehouse/typed_id"
)

type AgentProviderMaintenanceTarget struct {
	ID          string
	Fingerprint string
}

type AgentProviderMaintenanceState struct {
	Provider    SystemAgentProvider
	Fingerprint string
}

// Snapshot references only. This is safe as the dispatcher's DBOS step output.
func (store *Store) AgentProviderMaintenanceTargets(ctx context.Context) (error, []AgentProviderMaintenanceTarget) {
	rows, err := store.QueryContext(ctx, `SELECT id, protocol, keychain_id, keychain_version, credential FROM gatehouse_agent_providers WHERE enabled = TRUE ORDER BY id`)
	if err != nil {
		return err, nil
	}
	defer rows.Close()
	targets := []AgentProviderMaintenanceTarget{}
	for rows.Next() {
		var target AgentProviderMaintenanceTarget
		var protocol string
		var keychainID, encrypted sql.NullString
		var version sql.NullInt64
		if err := rows.Scan(&target.ID, &protocol, &keychainID, &version, &encrypted); err != nil {
			return err, nil
		}
		var ref *model.KeychainRef
		if keychainID.Valid && version.Valid {
			ref = &model.KeychainRef{Id: keychainID.String, Version: int(version.Int64)}
		}
		var credential *string
		if encrypted.Valid {
			credential = &encrypted.String
		}
		target.Fingerprint = agentProviderCredentialFingerprint(protocol, ref, credential)
		targets = append(targets, target)
	}
	return rows.Err(), targets
}

// Return public metadata plus an opaque reference. Ciphertext is hashed locally,
// never returned in the maintenance snapshot or decrypted for a freshness check.
func (store *Store) AgentProviderMaintenanceGet(ctx context.Context, id string) (error, *AgentProviderMaintenanceState) {
	if !typed_id.Valid(typed_id.AgentProvider, id) {
		return nil, nil
	}
	var encrypted sql.NullString
	provider, err := scanSystemAgentProvider(store.QueryRowContext(ctx, `SELECT id, alias, revision, protocol, endpoint, keychain_id, keychain_version, credential IS NOT NULL, enabled, credential_metadata, credential
		FROM gatehouse_agent_providers WHERE id = `+keychainPlaceholder(store.kind)(1), id), &encrypted)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return err, nil
	}
	record := AgentProviderCredential{Provider: provider}
	if encrypted.Valid {
		record.Encrypted = &encrypted.String
	}
	return nil, &AgentProviderMaintenanceState{Provider: provider, Fingerprint: record.Fingerprint()}
}

// Runtime maintenance changes credentials without changing configuration revision.
// Compare the actual ciphertext originally read, so a late refresh/rejection
// cannot replace a reconnect, undo a disconnect, or cross an encryption context.
func (store *Store) AgentProviderMaintenanceCredentialSet(ctx context.Context, id string, original string, ref model.KeychainRef, encrypted *string, account *SystemAgentProviderAccount) (error, bool) {
	if !typed_id.Valid(typed_id.AgentProvider, id) || strings.TrimSpace(original) == "" || ref.Id == "" || ref.Version < 1 {
		return fmt.Errorf("invalid maintenance credential update"), false
	}
	var metadata any
	if encrypted != nil {
		if strings.TrimSpace(*encrypted) == "" || account == nil || strings.TrimSpace(account.ID) == "" {
			return fmt.Errorf("invalid maintenance credential"), false
		}
		encoded, err := json.Marshal(account)
		if err != nil {
			return fmt.Errorf("encode provider account metadata"), false
		}
		metadata = string(encoded)
	} else if account != nil {
		return fmt.Errorf("invalid maintenance invalidation"), false
	}
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return err, false
	}
	defer tx.Rollback()
	p := keychainPlaceholder(store.kind)
	result, err := tx.ExecContext(ctx, `UPDATE gatehouse_agent_providers SET credential = `+p(1)+`, credential_metadata = `+p(2)+`
		WHERE id = `+p(3)+` AND credential = `+p(4)+` AND protocol = 'openai-codex' AND keychain_id = `+p(5)+` AND keychain_version = `+p(6), optionalStringValue(encrypted), metadata, id, original, ref.Id, ref.Version)
	if err != nil {
		return err, false
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return err, false
	}
	if err, _ := store.ActivityEventAppend(ctx, tx, model.ActivityEvent{Event: "agent_provider.update", ResourceKind: ActivityResourceKindAgentProvider, ResourceAgentProvider: &id}, []string{"sys/" + id}); err != nil {
		return err, false
	}
	if err := tx.Commit(); err != nil {
		return err, false
	}
	return nil, true
}
