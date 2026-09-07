package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"gatehouse/model"
	"gatehouse/typed_id"
)

var (
	ErrStorageProviderExists          = errors.New("storage provider already exists")
	ErrWorkspaceStorageProviderExists = errors.New("workspace storage provider already exists")
)

type SystemStorageProvider struct {
	ID                   string             `json:"id"`
	Alias                string             `json:"alias"`
	Revision             int                `json:"revision"`
	Protocol             string             `json:"protocol"`
	Endpoint             *string            `json:"endpoint,omitempty"`
	Region               *string            `json:"region,omitempty"`
	Bucket               *string            `json:"bucket,omitempty"`
	AccessKeyID          *string            `json:"access_key_id,omitempty"`
	Keychain             *model.KeychainRef `json:"keychain,omitempty"`
	CredentialConfigured bool               `json:"credential_configured"`
	Enabled              bool               `json:"enabled"`
}

type SystemStorageProviderInput struct {
	Alias                    string
	Protocol                 string
	Endpoint                 *string
	Region                   *string
	Bucket                   *string
	AccessKeyID              *string
	Keychain                 *model.KeychainRef
	EncryptedSecretAccessKey *string
	Enabled                  bool
}

type SystemWorkspaceStorageProvider struct {
	WorkspaceID string `json:"workspace"`
	ProviderID  string `json:"provider"`
	Revision    int    `json:"revision"`
	Priority    int    `json:"priority"`
	Enabled     bool   `json:"enabled"`
}

type SystemWorkspaceStorageProviderInput struct {
	Priority int
	Enabled  bool
}

func (store *Store) SystemStorageProvidersGet(ctx context.Context) (error, []SystemStorageProvider) {
	rows, err := store.QueryContext(ctx, `SELECT id, alias, revision, protocol, endpoint, region, bucket, access_key_id, keychain_id, keychain_version, secret_access_key IS NOT NULL, enabled FROM gatehouse_storage_providers ORDER BY id`)
	if err != nil { return fmt.Errorf("list system storage providers: %w", err), nil }
	defer rows.Close()
	providers := []SystemStorageProvider{}
	for rows.Next() { provider, err := scanSystemStorageProvider(rows); if err != nil { return err, nil }; providers = append(providers, provider) }
	if err := rows.Err(); err != nil { return fmt.Errorf("iterate system storage providers: %w", err), nil }
	return nil, providers
}

func (store *Store) SystemStorageProviderGet(ctx context.Context, id string) (error, *SystemStorageProvider) {
	if !typed_id.Valid(typed_id.StorageProvider, id) { return nil, nil }
	provider, err := scanSystemStorageProvider(store.QueryRowContext(ctx, `SELECT id, alias, revision, protocol, endpoint, region, bucket, access_key_id, keychain_id, keychain_version, secret_access_key IS NOT NULL, enabled FROM gatehouse_storage_providers WHERE id = `+keychainPlaceholder(store.kind)(1), id))
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return fmt.Errorf("get system storage provider: %w", err), nil }
	return nil, &provider
}

func (store *Store) SystemStorageProviderCreate(ctx context.Context, input SystemStorageProviderInput) (error, *SystemStorageProvider) {
	if err := validateSystemStorageProviderInput(input, false); err != nil { return err, nil }
	id, err := typed_id.New(typed_id.StorageProvider)
	if err != nil { return fmt.Errorf("generate storage provider ID: %w", err), nil }
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin system storage provider creation: %w", err), nil }
	defer transaction.Rollback()
	if err := store.systemKeychainExists(ctx, transaction, input.Keychain); err != nil { return err, nil }
	placeholder := keychainPlaceholder(store.kind)
	var inserted string
	err = transaction.QueryRowContext(ctx, `INSERT INTO gatehouse_storage_providers (id, alias, revision, protocol, endpoint, region, bucket, access_key_id, keychain_id, keychain_version, secret_access_key, enabled) VALUES (`+placeholder(1)+`, `+placeholder(2)+`, 1, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`, `+placeholder(10)+`, `+placeholder(11)+`) ON CONFLICT (alias) DO NOTHING RETURNING id`, id, input.Alias, input.Protocol, optionalStringValue(input.Endpoint), optionalStringValue(input.Region), optionalStringValue(input.Bucket), optionalStringValue(input.AccessKeyID), keychainID(input.Keychain), keychainVersion(input.Keychain), optionalStringValue(input.EncryptedSecretAccessKey), input.Enabled).Scan(&inserted)
	if err == sql.ErrNoRows { return ErrStorageProviderExists, nil }
	if err != nil { return fmt.Errorf("insert system storage provider: %w", err), nil }
	if err, _ := store.ActivityEventAppend(ctx, transaction, storageProviderActivity("storage_provider.create", id), []string{"sys/" + id}); err != nil { return fmt.Errorf("append system storage provider creation activity: %w", err), nil }
	if err := transaction.Commit(); err != nil { return fmt.Errorf("commit system storage provider creation: %w", err), nil }
	return store.SystemStorageProviderGet(ctx, id)
}

func (store *Store) SystemStorageProviderUpdate(ctx context.Context, id string, expectedRevision int, input SystemStorageProviderInput) (error, *SystemStorageProvider) {
	if !typed_id.Valid(typed_id.StorageProvider, id) || expectedRevision < 1 { return nil, nil }
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin system storage provider update: %w", err), nil }
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	var alias, protocol string
	var keychainIDValue sql.NullString
	var keychainVersionValue sql.NullInt64
	var encrypted sql.NullString
	var revision int
	err = transaction.QueryRowContext(ctx, `SELECT alias, revision, protocol, keychain_id, keychain_version, secret_access_key FROM gatehouse_storage_providers WHERE id = `+placeholder(1), id).Scan(&alias, &revision, &protocol, &keychainIDValue, &keychainVersionValue, &encrypted)
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return fmt.Errorf("get system storage provider for update: %w", err), nil }
	if revision != expectedRevision { return ErrRevisionConflict, nil }
	if input.Alias != alias { return fmt.Errorf("update system storage provider: alias is immutable"), nil }
	if input.EncryptedSecretAccessKey == nil && input.Protocol != "embedded" && (protocol != input.Protocol || input.Keychain == nil || !keychainIDValue.Valid || input.Keychain.Id != keychainIDValue.String || int(keychainVersionValue.Int64) != input.Keychain.Version) { return fmt.Errorf("update system storage provider: credential replacement is required"), nil }
	if err := validateSystemStorageProviderInput(input, input.EncryptedSecretAccessKey == nil); err != nil { return err, nil }
	if err := store.systemKeychainExists(ctx, transaction, input.Keychain); err != nil { return err, nil }
	credential := optionalStringValue(input.EncryptedSecretAccessKey)
	if input.EncryptedSecretAccessKey == nil && encrypted.Valid { credential = encrypted.String }
	if input.Protocol == "embedded" { credential = nil }
	result, err := transaction.ExecContext(ctx, `UPDATE gatehouse_storage_providers SET revision = `+placeholder(1)+`, protocol = `+placeholder(2)+`, endpoint = `+placeholder(3)+`, region = `+placeholder(4)+`, bucket = `+placeholder(5)+`, access_key_id = `+placeholder(6)+`, keychain_id = `+placeholder(7)+`, keychain_version = `+placeholder(8)+`, secret_access_key = `+placeholder(9)+`, enabled = `+placeholder(10)+` WHERE id = `+placeholder(11)+` AND revision = `+placeholder(12), revision+1, input.Protocol, optionalStringValue(input.Endpoint), optionalStringValue(input.Region), optionalStringValue(input.Bucket), optionalStringValue(input.AccessKeyID), keychainID(input.Keychain), keychainVersion(input.Keychain), credential, input.Enabled, id, revision)
	if err != nil { return fmt.Errorf("update system storage provider: %w", err), nil }
	count, err := result.RowsAffected(); if err != nil { return fmt.Errorf("count system storage provider update: %w", err), nil }; if count == 0 { return ErrRevisionConflict, nil }
	if err, _ := store.ActivityEventAppend(ctx, transaction, storageProviderActivity("storage_provider.update", id), []string{"sys/" + id}); err != nil { return fmt.Errorf("append system storage provider update activity: %w", err), nil }
	if err := transaction.Commit(); err != nil { return fmt.Errorf("commit system storage provider update: %w", err), nil }
	return store.SystemStorageProviderGet(ctx, id)
}

func (store *Store) SystemWorkspaceStorageProvidersGet(ctx context.Context) (error, []SystemWorkspaceStorageProvider) {
	rows, err := store.QueryContext(ctx, `SELECT workspace, provider, revision, priority, enabled FROM gatehouse_workspace_storage_providers ORDER BY workspace, provider`)
	if err != nil { return fmt.Errorf("list system workspace storage providers: %w", err), nil }
	defer rows.Close()
	bindings := []SystemWorkspaceStorageProvider{}
	for rows.Next() { binding, err := scanSystemWorkspaceStorageProvider(rows); if err != nil { return err, nil }; bindings = append(bindings, binding) }
	if err := rows.Err(); err != nil { return fmt.Errorf("iterate system workspace storage providers: %w", err), nil }
	return nil, bindings
}

func (store *Store) SystemWorkspaceStorageProviderGet(ctx context.Context, workspaceID, providerID string) (error, *SystemWorkspaceStorageProvider) {
	if !typed_id.Valid(typed_id.Workspace, workspaceID) || !typed_id.Valid(typed_id.StorageProvider, providerID) { return nil, nil }
	binding, err := scanSystemWorkspaceStorageProvider(store.QueryRowContext(ctx, `SELECT workspace, provider, revision, priority, enabled FROM gatehouse_workspace_storage_providers WHERE workspace = `+keychainPlaceholder(store.kind)(1)+` AND provider = `+keychainPlaceholder(store.kind)(2), workspaceID, providerID))
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return fmt.Errorf("get system workspace storage provider: %w", err), nil }
	return nil, &binding
}

func (store *Store) SystemWorkspaceStorageProviderCreate(ctx context.Context, workspaceID, providerID string, input SystemWorkspaceStorageProviderInput) (error, *SystemWorkspaceStorageProvider) {
	if !typed_id.Valid(typed_id.Workspace, workspaceID) || !typed_id.Valid(typed_id.StorageProvider, providerID) || input.Priority < 1 { return nil, nil }
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin system workspace storage provider creation: %w", err), nil }
	defer transaction.Rollback()
	if err := store.systemWorkspaceAndStorageProviderExist(ctx, transaction, workspaceID, providerID); err != nil { return err, nil }
	placeholder := keychainPlaceholder(store.kind)
	result, err := transaction.ExecContext(ctx, `INSERT INTO gatehouse_workspace_storage_providers (workspace, provider, revision, priority, enabled) VALUES (`+placeholder(1)+`, `+placeholder(2)+`, 1, `+placeholder(3)+`, `+placeholder(4)+`) ON CONFLICT (workspace, provider) DO NOTHING`, workspaceID, providerID, input.Priority, input.Enabled)
	if err != nil { return fmt.Errorf("insert system workspace storage provider: %w", err), nil }
	count, err := result.RowsAffected(); if err != nil { return fmt.Errorf("count system workspace storage provider creation: %w", err), nil }; if count == 0 { return ErrWorkspaceStorageProviderExists, nil }
	if err, _ := store.ActivityEventAppend(ctx, transaction, workspaceStorageProviderActivity("workspace_storage_provider.create", workspaceID, providerID), []string{workspaceID}); err != nil { return fmt.Errorf("append system workspace storage provider creation activity: %w", err), nil }
	if err := transaction.Commit(); err != nil { return fmt.Errorf("commit system workspace storage provider creation: %w", err), nil }
	return store.SystemWorkspaceStorageProviderGet(ctx, workspaceID, providerID)
}

func (store *Store) SystemWorkspaceStorageProviderUpdate(ctx context.Context, workspaceID, providerID string, expectedRevision int, input SystemWorkspaceStorageProviderInput) (error, *SystemWorkspaceStorageProvider) {
	if !typed_id.Valid(typed_id.Workspace, workspaceID) || !typed_id.Valid(typed_id.StorageProvider, providerID) || expectedRevision < 1 || input.Priority < 1 { return nil, nil }
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin system workspace storage provider update: %w", err), nil }
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	var revision int
	err = transaction.QueryRowContext(ctx, `SELECT revision FROM gatehouse_workspace_storage_providers WHERE workspace = `+placeholder(1)+` AND provider = `+placeholder(2), workspaceID, providerID).Scan(&revision)
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return fmt.Errorf("get system workspace storage provider for update: %w", err), nil }
	if revision != expectedRevision { return ErrRevisionConflict, nil }
	result, err := transaction.ExecContext(ctx, `UPDATE gatehouse_workspace_storage_providers SET revision = `+placeholder(1)+`, priority = `+placeholder(2)+`, enabled = `+placeholder(3)+` WHERE workspace = `+placeholder(4)+` AND provider = `+placeholder(5)+` AND revision = `+placeholder(6), revision+1, input.Priority, input.Enabled, workspaceID, providerID, revision)
	if err != nil { return fmt.Errorf("update system workspace storage provider: %w", err), nil }
	count, err := result.RowsAffected(); if err != nil { return fmt.Errorf("count system workspace storage provider update: %w", err), nil }; if count == 0 { return ErrRevisionConflict, nil }
	if err, _ := store.ActivityEventAppend(ctx, transaction, workspaceStorageProviderActivity("workspace_storage_provider.update", workspaceID, providerID), []string{workspaceID}); err != nil { return fmt.Errorf("append system workspace storage provider update activity: %w", err), nil }
	if err := transaction.Commit(); err != nil { return fmt.Errorf("commit system workspace storage provider update: %w", err), nil }
	return store.SystemWorkspaceStorageProviderGet(ctx, workspaceID, providerID)
}

func scanSystemStorageProvider(row interface { Scan(...any) error }) (SystemStorageProvider, error) {
	var value SystemStorageProvider
	var endpoint, region, bucket, accessKeyID, keychainIDValue sql.NullString
	var keychainVersionValue sql.NullInt64
	if err := row.Scan(&value.ID, &value.Alias, &value.Revision, &value.Protocol, &endpoint, &region, &bucket, &accessKeyID, &keychainIDValue, &keychainVersionValue, &value.CredentialConfigured, &value.Enabled); err != nil { return SystemStorageProvider{}, err }
	if endpoint.Valid { value.Endpoint = &endpoint.String }; if region.Valid { value.Region = &region.String }; if bucket.Valid { value.Bucket = &bucket.String }; if accessKeyID.Valid { value.AccessKeyID = &accessKeyID.String }
	if keychainIDValue.Valid && keychainVersionValue.Valid { value.Keychain = &model.KeychainRef{Id: keychainIDValue.String, Version: int(keychainVersionValue.Int64)} }
	return value, nil
}

func scanSystemWorkspaceStorageProvider(row interface { Scan(...any) error }) (SystemWorkspaceStorageProvider, error) {
	var value SystemWorkspaceStorageProvider
	if err := row.Scan(&value.WorkspaceID, &value.ProviderID, &value.Revision, &value.Priority, &value.Enabled); err != nil { return SystemWorkspaceStorageProvider{}, err }
	return value, nil
}

func validateSystemStorageProviderInput(input SystemStorageProviderInput, credentialOptional bool) error {
	if !administrationAlias.MatchString(input.Alias) { return fmt.Errorf("system storage provider alias is invalid") }
	switch input.Protocol {
	case "embedded":
		if input.Endpoint != nil || input.Region != nil || input.Bucket != nil || input.AccessKeyID != nil || input.Keychain != nil || input.EncryptedSecretAccessKey != nil { return fmt.Errorf("embedded system storage provider credentials are invalid") }
	case "s3":
		if input.Endpoint == nil || input.Region == nil || input.Bucket == nil || input.AccessKeyID == nil || input.Keychain == nil || (!credentialOptional && input.EncryptedSecretAccessKey == nil) { return fmt.Errorf("system storage provider credentials are required") }
		parsed, err := url.ParseRequestURI(*input.Endpoint)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || strings.TrimSpace(*input.Region) == "" || strings.TrimSpace(*input.Bucket) == "" || strings.TrimSpace(*input.AccessKeyID) == "" || !administrationAlias.MatchString(input.Keychain.Id) || input.Keychain.Version < 1 { return fmt.Errorf("system storage provider configuration is invalid") }
	default:
		return fmt.Errorf("system storage provider protocol is invalid")
	}
	if input.EncryptedSecretAccessKey != nil && strings.TrimSpace(*input.EncryptedSecretAccessKey) == "" { return fmt.Errorf("system storage provider credential is invalid") }
	return nil
}

func (store *Store) systemWorkspaceAndStorageProviderExist(ctx context.Context, transaction *sql.Tx, workspaceID, providerID string) error {
	placeholder := keychainPlaceholder(store.kind)
	var stored string
	err := transaction.QueryRowContext(ctx, `SELECT id FROM gatehouse_workspaces WHERE id = `+placeholder(1), workspaceID).Scan(&stored)
	if err == sql.ErrNoRows { return ErrAdminDependencyMissing }; if err != nil { return fmt.Errorf("get system workspace: %w", err) }
	err = transaction.QueryRowContext(ctx, `SELECT id FROM gatehouse_storage_providers WHERE id = `+placeholder(1), providerID).Scan(&stored)
	if err == sql.ErrNoRows { return ErrAdminDependencyMissing }; if err != nil { return fmt.Errorf("get system storage provider: %w", err) }
	return nil
}

func storageProviderActivity(event, id string) model.ActivityEvent { return model.ActivityEvent{Event: event, ResourceKind: ActivityResourceKindStorageProvider, ResourceStorageProvider: &id} }
func workspaceStorageProviderActivity(event, workspaceID, providerID string) model.ActivityEvent { return model.ActivityEvent{Event: event, ResourceKind: ActivityResourceKindWorkspaceStorageProvider, ResourceWorkspaceStorageProviderWorkspace: &workspaceID, ResourceWorkspaceStorageProviderProvider: &providerID} }
