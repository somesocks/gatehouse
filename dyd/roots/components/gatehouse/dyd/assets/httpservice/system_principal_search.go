package httpservice

import (
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
	"net/http"
)

func systemPrincipalsSearch(w http.ResponseWriter, r *http.Request, store *database.Store) {
	name, cursor, limit, ok := catalogSearchParameters(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	if (cursor != "" && !typed_id.Valid(typed_id.Principal, cursor)) || (status != "" && status != "enabled" && status != "disabled") {
		http.Error(w, "invalid principal search", 400)
		return
	}
	err, principals, next := store.SystemPrincipalsSearch(r.Context(), name, cursor, status, limit)
	if err != nil {
		http.Error(w, "principals could not be loaded", 500)
		return
	}
	writeJSON(w, struct {
		Principals []database.SystemPrincipal `json:"principals"`
		NextCursor string                     `json:"next_cursor,omitempty"`
	}{principals, next})
}
func systemGrantsSearch(w http.ResponseWriter, r *http.Request, store *database.Store) {
	name, cursor, limit, ok := catalogSearchParameters(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	if (cursor != "" && !typed_id.Valid(typed_id.SystemGrant, cursor)) || (status != "" && status != "enabled" && status != "disabled") {
		http.Error(w, "invalid grant search", 400)
		return
	}
	err, grants, principals, next := store.SystemGrantsSearch(r.Context(), name, cursor, status, limit)
	if err != nil {
		http.Error(w, "grants could not be loaded", 500)
		return
	}
	writeJSON(w, struct {
		Grants     []model.SystemGrant        `json:"grants"`
		Principals []database.SystemPrincipal `json:"principals"`
		NextCursor string                     `json:"next_cursor,omitempty"`
	}{grants, principals, next})
}
