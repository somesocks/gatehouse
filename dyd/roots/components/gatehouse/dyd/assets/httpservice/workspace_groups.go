package httpservice

import (
	"net/http"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func groupRequestScope(w http.ResponseWriter, r *http.Request, store *database.Store, tokens *auth.BearerTokens, system, manage bool) (*model.PrincipalRef, bool) {
	if system {
		return nil, systemGroupScope(w, r, store, tokens, true)
	}
	claims, ok := authenticate(w, r, tokens)
	if !ok {
		return nil, false
	}
	workspace, ok := authorizedWorkspace(w, r, store, claims)
	if !ok {
		return nil, false
	}
	err, role := store.GroupRoleGet(r.Context(), workspace.Id, r.PathValue("group"), claims.Principal.Ref)
	if err != nil {
		groupAdminError(w, r, err)
		return nil, false
	}
	if role == "" {
		http.NotFound(w, r)
		return nil, false
	}
	if manage && !authz.GroupAllows([]authz.Role{role}, authz.GroupMembershipManage) {
		groupAdminError(w, r, database.ErrGroupForbidden)
		return nil, false
	}
	return &claims.Principal.Ref, true
}

func workspaceGroupCreate(w http.ResponseWriter, r *http.Request, store *database.Store, tokens *auth.BearerTokens) {
	claims, ok := authenticate(w, r, tokens)
	if !ok {
		return
	}
	workspace, roles, ok := authorizedWorkspaceRoles(w, r, store, claims)
	if !ok || !workspaceActionAllowed(w, roles, authz.WorkspaceGroupCreate) {
		return
	}
	var input systemGroupRequest
	if !principalDecode(w, r, &input) {
		return
	}
	if input.ExpectedRevision != nil {
		http.Error(w, "new groups do not accept a revision", 400)
		return
	}
	err, group := store.GroupCreate(r.Context(), workspace.Id, database.SystemGroupInput{Alias: input.Alias, Name: input.Name, Enabled: input.Enabled}, &claims.Principal.Ref)
	if err != nil {
		groupAdminError(w, r, err)
		return
	}
	if group == nil {
		http.NotFound(w, r)
		return
	}
	group.Role = authz.Manager
	writeJSONStatus(w, http.StatusCreated, group)
}

func workspaceGroupCapabilities(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			return
		}
		claims, ok := authenticate(w, r, tokens)
		if !ok {
			return
		}
		_, roles, ok := authorizedWorkspaceRoles(w, r, store, claims)
		if !ok {
			return
		}
		writeJSON(w, struct {
			Create bool `json:"create"`
		}{authz.WorkspaceAllows(roles, authz.WorkspaceGroupCreate)})
	}
}

func groupAgentOptions(store *database.Store, tokens *auth.BearerTokens, system bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			return
		}
		if _, ok := groupRequestScope(w, r, store, tokens, system, true); !ok {
			return
		}
		name, cursor, limit, ok := catalogSearchParameters(w, r)
		if !ok {
			return
		}
		if cursor != "" && !typed_id.Valid(typed_id.WorkspaceAgent, cursor) {
			http.Error(w, "invalid bot cursor", 400)
			return
		}
		err, agents, next := store.SystemWorkspaceAgentsSearch(r.Context(), database.SystemWorkspaceAgentSearch{WorkspaceID: r.PathValue("workspace"), Name: name, Cursor: cursor, Limit: limit, ActiveOnly: true})
		if err != nil {
			groupAdminError(w, r, err)
			return
		}
		// Options expose only the workspace-local bot identity and display data.
		type option struct {
			ID      string  `json:"id"`
			Alias   string  `json:"alias"`
			Label   *string `json:"label,omitempty"`
			Enabled bool    `json:"enabled"`
		}
		items := make([]option, 0, len(agents))
		for _, agent := range agents {
			items = append(items, option{agent.ID, agent.Alias, agent.Label, agent.Enabled})
		}
		writeJSON(w, struct {
			Agents []option `json:"agents"`
			Next   string   `json:"next_cursor,omitempty"`
		}{items, next})
	}
}
