package httpservice

import (
	"net/http"

	"gatehouse/auth"
	"gatehouse/database"
)

func systemGroupAgents(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return groupAgents(store, tokens, true)
}

func groupAgents(store *database.Store, tokens *auth.BearerTokens, system bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		actor, ok := groupRequestScope(w, r, store, tokens, system, r.Method != http.MethodGet)
		if !ok {
			return
		}
		workspace, group := r.PathValue("workspace"), r.PathValue("group")
		if r.Method == http.MethodGet {
			name, cursor, limit, ok := catalogSearchParameters(w, r)
			if !ok {
				return
			}
			err, agents, next := store.SystemGroupAgentsSearch(r.Context(), database.SystemGroupSearch{Workspace: workspace, Group: group, Name: name, Cursor: cursor, Limit: limit, Status: r.URL.Query().Get("status"), ActiveOnly: !system || r.URL.Query().Get("active") == "true"})
			if err != nil {
				groupAdminError(w, r, err)
				return
			}
			writeJSON(w, struct {
				Agents     []database.SystemGroupAgent `json:"agents"`
				NextCursor string                      `json:"next_cursor,omitempty"`
			}{agents, next})
			return
		}
		var input struct {
			Agent string `json:"agent"`
		}
		if !principalDecode(w, r, &input) {
			return
		}
		if err := store.GroupAgentCreate(r.Context(), workspace, group, input.Agent, actor); err != nil {
			groupAdminError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

func systemGroupAgent(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return groupAgent(store, tokens, true)
}

func groupAgent(store *database.Store, tokens *auth.BearerTokens, system bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		actor, ok := groupRequestScope(w, r, store, tokens, system, true)
		if !ok {
			return
		}
		var input struct {
			Enabled          *bool `json:"enabled"`
			ExpectedRevision *int  `json:"expected_revision"`
		}
		if !principalDecode(w, r, &input) {
			return
		}
		if input.Enabled == nil || input.ExpectedRevision == nil {
			http.Error(w, "enabled and expected revision are required", http.StatusBadRequest)
			return
		}
		if err := store.GroupAgentUpdate(r.Context(), r.PathValue("workspace"), r.PathValue("group"), r.PathValue("agent"), *input.Enabled, *input.ExpectedRevision, actor); err != nil {
			groupAdminError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
