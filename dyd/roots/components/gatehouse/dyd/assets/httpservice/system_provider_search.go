package httpservice

import (
	"gatehouse/database"
	"gatehouse/typed_id"
	"net/http"
)

func systemAgentProvidersSearch(w http.ResponseWriter, r *http.Request, store *database.Store) {
	name, cursor, limit, ok := catalogSearchParameters(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	if (cursor != "" && !typed_id.Valid(typed_id.AgentProvider, cursor)) || (status != "" && status != "enabled" && status != "disabled") {
		http.Error(w, "invalid provider search", 400)
		return
	}
	err, providers, next := store.SystemAgentProvidersSearch(r.Context(), name, cursor, status, limit)
	if err != nil {
		http.Error(w, "providers could not be loaded", 500)
		return
	}
	writeJSON(w, struct {
		Providers  []database.SystemAgentProvider `json:"providers"`
		NextCursor string                         `json:"next_cursor,omitempty"`
	}{providers, next})
}
func systemAgentModelsSearch(w http.ResponseWriter, r *http.Request, store *database.Store) {
	name, cursor, limit, ok := catalogSearchParameters(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	if (cursor != "" && !typed_id.Valid(typed_id.AgentModel, cursor)) || (status != "" && status != "enabled" && status != "disabled") {
		http.Error(w, "invalid model search", 400)
		return
	}
	err, models, next := store.SystemAgentModelsSearch(r.Context(), name, cursor, status, limit)
	if err != nil {
		http.Error(w, "models could not be loaded", 500)
		return
	}
	writeJSON(w, struct {
		Models     []database.SystemAgentModel `json:"models"`
		NextCursor string                      `json:"next_cursor,omitempty"`
	}{models, next})
}
func systemStorageProvidersSearch(w http.ResponseWriter, r *http.Request, store *database.Store) {
	name, cursor, limit, ok := catalogSearchParameters(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	if (cursor != "" && !typed_id.Valid(typed_id.StorageProvider, cursor)) || (status != "" && status != "enabled" && status != "disabled") {
		http.Error(w, "invalid storage provider search", 400)
		return
	}
	err, providers, next := store.SystemStorageProvidersSearch(r.Context(), name, cursor, status, limit)
	if err != nil {
		http.Error(w, "providers could not be loaded", 500)
		return
	}
	writeJSON(w, struct {
		Providers  []database.SystemStorageProvider `json:"providers"`
		NextCursor string                           `json:"next_cursor,omitempty"`
	}{providers, next})
}
