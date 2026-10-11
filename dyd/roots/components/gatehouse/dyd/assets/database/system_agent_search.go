package database

import (
	"context"
	"fmt"
	"gatehouse/typed_id"
	"strings"
)

func (store *Store) SystemAgentProvidersSearch(ctx context.Context, name, cursor, status string, limit int) (error, []SystemAgentProvider, string) {
	if limit < 1 || limit > 100 || len(name) > 256 || (cursor != "" && !typed_id.Valid(typed_id.AgentProvider, cursor)) || (status != "" && status != "enabled" && status != "disabled") {
		return fmt.Errorf("invalid agent provider search"), nil, ""
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(name)) + "%"
	p := keychainPlaceholder(store.kind)
	query := `SELECT id, alias, revision, protocol, base_url, keychain_id, keychain_version, api_key IS NOT NULL, enabled FROM gatehouse_agent_providers WHERE (LOWER(alias) LIKE LOWER(` + p(1) + `) ESCAPE '\' OR LOWER(id) LIKE LOWER(` + p(2) + `) ESCAPE '\' OR LOWER(protocol) LIKE LOWER(` + p(3) + `) ESCAPE '\') AND (` + p(4) + ` = '' OR id < ` + p(5) + `)`
	args := []any{pattern, pattern, pattern, cursor, cursor}
	if status != "" {
		query += ` AND enabled = ` + p(6)
		args = append(args, status == "enabled")
	}
	query += ` ORDER BY id DESC LIMIT ` + p(len(args)+1)
	args = append(args, limit+1)
	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return err, nil, ""
	}
	defer rows.Close()
	providers := []SystemAgentProvider{}
	for rows.Next() {
		provider, err := scanSystemAgentProvider(rows)
		if err != nil {
			return err, nil, ""
		}
		providers = append(providers, provider)
	}
	if err := rows.Err(); err != nil {
		return err, nil, ""
	}
	if len(providers) <= limit {
		return nil, providers, ""
	}
	return nil, providers[:limit], providers[limit-1].ID
}

func (store *Store) SystemAgentModelsSearch(ctx context.Context, name, cursor, status string, limit int) (error, []SystemAgentModel, string) {
	if limit < 1 || limit > 100 || len(name) > 256 || (cursor != "" && !typed_id.Valid(typed_id.AgentModel, cursor)) || (status != "" && status != "enabled" && status != "disabled") {
		return fmt.Errorf("invalid agent model search"), nil, ""
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(name)) + "%"
	p := keychainPlaceholder(store.kind)
	query := `SELECT models.id, models.alias, models.revision, models.provider_id, models.model, models.parameters, models.compaction, models.max_turns, models.max_output_tokens, models.enabled, providers.alias FROM gatehouse_agent_models AS models JOIN gatehouse_agent_providers AS providers ON providers.id = models.provider_id WHERE (LOWER(models.alias) LIKE LOWER(` + p(1) + `) ESCAPE '\' OR LOWER(models.id) LIKE LOWER(` + p(2) + `) ESCAPE '\' OR LOWER(models.model) LIKE LOWER(` + p(3) + `) ESCAPE '\' OR LOWER(providers.alias) LIKE LOWER(` + p(4) + `) ESCAPE '\') AND (` + p(5) + ` = '' OR models.id < ` + p(6) + `)`
	args := []any{pattern, pattern, pattern, pattern, cursor, cursor}
	if status != "" {
		query += ` AND models.enabled = ` + p(7)
		args = append(args, status == "enabled")
	}
	query += ` ORDER BY models.id DESC LIMIT ` + p(len(args)+1)
	args = append(args, limit+1)
	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return err, nil, ""
	}
	defer rows.Close()
	models := []SystemAgentModel{}
	for rows.Next() {
		var model SystemAgentModel
		var alias string
		if err := rows.Scan(&model.ID, &model.Alias, &model.Revision, &model.ProviderID, &model.Model, &model.Parameters, &model.Compaction, &model.MaxTurns, &model.MaxOutputTokens, &model.Enabled, &alias); err != nil {
			return err, nil, ""
		}
		model.ProviderAlias = &alias
		models = append(models, model)
	}
	if err := rows.Err(); err != nil {
		return err, nil, ""
	}
	if len(models) <= limit {
		return nil, models, ""
	}
	return nil, models[:limit], models[limit-1].ID
}
