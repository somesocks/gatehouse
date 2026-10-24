package httpservice

import (
	"errors"
	"net/http"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/database"
)

func systemGroupScope(w http.ResponseWriter, r *http.Request, store *database.Store, tokens *auth.BearerTokens, group bool) bool {
	if !systemManage(w, r, store, tokens) {
		return false
	}
	err, workspace := store.SystemWorkspaceGet(r.Context(), r.PathValue("workspace"))
	if err != nil {
		groupAdminError(w, r, err)
		return false
	}
	if workspace == nil {
		http.NotFound(w, r)
		return false
	}
	if group {
		err, record := store.SystemGroupGet(r.Context(), r.PathValue("workspace"), r.PathValue("group"))
		if err != nil {
			groupAdminError(w, r, err)
			return false
		}
		if record == nil {
			http.NotFound(w, r)
			return false
		}
	}
	return true
}

func groupAdminError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, database.ErrGroupForbidden):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, database.ErrRevisionConflict):
		http.Error(w, "This record changed elsewhere. Reload before retrying.", http.StatusConflict)
	case errors.Is(err, database.ErrGroupLastAdmin):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, database.ErrGroupAliasExists), errors.Is(err, database.ErrGroupMemberExists), errors.Is(err, database.ErrWorkspaceGrantAlreadyExists):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, database.ErrGroupInput), errors.Is(err, database.ErrWorkspaceGrantRoleInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, database.ErrGroupNotFound), errors.Is(err, database.ErrGroupPrincipalNotFound), errors.Is(err, database.ErrGroupAgentNotFound):
		http.NotFound(w, r)
	default:
		http.Error(w, "Group administration request could not be completed.", http.StatusInternalServerError)
	}
}

type systemGroupRequest struct {
	Alias            *string `json:"alias"`
	Name             *string `json:"name"`
	Enabled          *bool   `json:"enabled"`
	ExpectedRevision *int    `json:"expected_revision"`
}

func systemGroups(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		if !systemGroupScope(w, r, store, tokens, false) {
			return
		}
		workspace := r.PathValue("workspace")
		if r.Method == http.MethodGet {
			name, cursor, limit, ok := catalogSearchParameters(w, r)
			if !ok {
				return
			}
			err, groups, next := store.SystemGroupsSearch(r.Context(), database.SystemGroupSearch{Workspace: workspace, Name: name, Cursor: cursor, Limit: limit, Status: r.URL.Query().Get("status")})
			if err != nil {
				groupAdminError(w, r, err)
				return
			}
			writeJSON(w, struct {
				Groups     []database.SystemGroup `json:"groups"`
				NextCursor string                 `json:"next_cursor,omitempty"`
			}{groups, next})
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
		err, group := store.SystemGroupCreate(r.Context(), workspace, database.SystemGroupInput{Alias: input.Alias, Name: input.Name, Enabled: input.Enabled})
		if err != nil {
			groupAdminError(w, r, err)
			return
		}
		if group == nil {
			http.NotFound(w, r)
			return
		}
		writeJSONStatus(w, http.StatusCreated, group)
	}
}

func systemGroup(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return groupDetail(store, tokens, true)
}

func groupDetail(store *database.Store, tokens *auth.BearerTokens, system bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPatch {
			w.WriteHeader(405)
			return
		}
		actor, ok := groupRequestScope(w, r, store, tokens, system, r.Method != http.MethodGet)
		if !ok {
			return
		}
		workspace, id := r.PathValue("workspace"), r.PathValue("group")
		if r.Method == http.MethodGet {
			err, group := store.SystemGroupGet(r.Context(), workspace, id)
			if err != nil {
				groupAdminError(w, r, err)
				return
			}
			if group == nil {
				http.NotFound(w, r)
				return
			}
			if actor != nil {
				err, group.Role = store.GroupRoleGet(r.Context(), workspace, id, *actor)
				if err != nil {
					groupAdminError(w, r, err)
					return
				}
				if group.Role == "" {
					http.NotFound(w, r)
					return
				}
			}
			writeJSON(w, group)
			return
		}
		var input systemGroupRequest
		if !principalDecode(w, r, &input) {
			return
		}
		if input.ExpectedRevision == nil {
			http.Error(w, "expected revision is required", 400)
			return
		}
		err, group := store.GroupUpdate(r.Context(), workspace, id, *input.ExpectedRevision, database.SystemGroupInput{Alias: input.Alias, Name: input.Name, Enabled: input.Enabled}, actor)
		if err != nil {
			groupAdminError(w, r, err)
			return
		}
		if group == nil {
			http.NotFound(w, r)
			return
		}
		if actor != nil {
			group.Role = authz.Admin
		}
		writeJSON(w, group)
	}
}

func systemGroupMembers(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return groupMembers(store, tokens, true)
}

func groupMembers(store *database.Store, tokens *auth.BearerTokens, system bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.WriteHeader(405)
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
			err, members, next := store.SystemGroupMembersSearch(r.Context(), database.SystemGroupSearch{Workspace: workspace, Group: group, Name: name, Cursor: cursor, Limit: limit, Status: r.URL.Query().Get("status"), ActiveOnly: !system || r.URL.Query().Get("active") == "true"})
			if err != nil {
				groupAdminError(w, r, err)
				return
			}
			writeJSON(w, struct {
				Members    []database.SystemGroupMember `json:"members"`
				NextCursor string                       `json:"next_cursor,omitempty"`
			}{members, next})
			return
		}
		var input struct {
			Principal string     `json:"principal"`
			Role      authz.Role `json:"role"`
		}
		if !principalDecode(w, r, &input) {
			return
		}
		if input.Role == "" {
			input.Role = authz.Member
		}
		if err := store.GroupMemberCreate(r.Context(), workspace, group, input.Principal, input.Role, actor); err != nil {
			groupAdminError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

func systemGroupMember(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return groupMember(store, tokens, true)
}

func groupMember(store *database.Store, tokens *auth.BearerTokens, system bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.WriteHeader(405)
			return
		}
		actor, ok := groupRequestScope(w, r, store, tokens, system, true)
		if !ok {
			return
		}
		var input struct {
			Enabled          *bool       `json:"enabled"`
			Role             *authz.Role `json:"role"`
			ExpectedRevision *int        `json:"expected_revision"`
		}
		if !principalDecode(w, r, &input) {
			return
		}
		if (input.Enabled == nil && input.Role == nil) || input.ExpectedRevision == nil {
			http.Error(w, "enabled or role, and expected revision are required", 400)
			return
		}
		if err := store.GroupMemberUpdate(r.Context(), r.PathValue("workspace"), r.PathValue("group"), r.PathValue("principal"), input.Enabled, input.Role, *input.ExpectedRevision, actor); err != nil {
			groupAdminError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func systemGroupMemberOptions(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return groupMemberOptions(store, tokens, true)
}

func groupMemberOptions(store *database.Store, tokens *auth.BearerTokens, system bool) http.HandlerFunc {
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
		err, people, next := store.SystemWorkspacePrincipalOptionsSearch(r.Context(), database.SystemWorkspacePrincipalSearch{WorkspaceID: r.PathValue("workspace"), Name: name, Cursor: cursor, Limit: limit, AdmittedOnly: true})
		if err != nil {
			groupAdminError(w, r, err)
			return
		}
		writeJSON(w, struct {
			Principals []database.SystemWorkspacePrincipalOption `json:"principals"`
			Next       string                                    `json:"next_cursor,omitempty"`
		}{people, next})
	}
}
