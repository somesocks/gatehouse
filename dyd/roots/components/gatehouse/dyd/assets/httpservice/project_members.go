package httpservice

import (
	"errors"
	"net/http"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/database"
)

func projectMemberError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, database.ErrProjectMemberInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, database.ErrProjectMemberNotFound):
		http.NotFound(w, r)
	case errors.Is(err, database.ErrProjectMemberForbidden):
		http.Error(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, database.ErrProjectGrantExists), errors.Is(err, database.ErrProjectLastAdmin):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, database.ErrRevisionConflict):
		http.Error(w, "This grant changed elsewhere. Reload before retrying.", http.StatusConflict)
	default:
		http.Error(w, "Project access could not be updated.", http.StatusInternalServerError)
	}
}

func workspaceProjectMembers(store *database.Store, tokens *auth.BearerTokens, optionsKind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(w, r, tokens)
		if !ok {
			return
		}
		project, ok := authorizedProject(w, r, store, claims)
		if !ok {
			return
		}
		noStore(w)
		err, roles := store.ProjectRolesGet(r.Context(), project, claims.Principal.Ref)
		if err != nil {
			projectMemberError(w, r, err)
			return
		}
		canManage := authz.ProjectAllows(roles, authz.ProjectGrantManage)
		if optionsKind != "" && !canManage {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		name, cursor, limit, ok := catalogSearchParameters(w, r)
		if !ok {
			return
		}
		kind := r.URL.Query().Get("kind")
		if optionsKind != "" {
			kind = optionsKind
		}
		if kind == "" {
			kind = "principal"
		}
		err, items, next := store.ProjectMembersSearch(r.Context(), project, claims.Principal.Ref, database.ProjectMemberSearch{Kind: kind, Name: name, Cursor: cursor, Limit: limit}, optionsKind != "")
		if err != nil {
			projectMemberError(w, r, err)
			return
		}
		writeJSON(w, struct {
			Members    []database.ProjectMember `json:"members"`
			NextCursor string                   `json:"next_cursor,omitempty"`
			CanManage  bool                     `json:"can_manage_access"`
		}{items, next, canManage})
	}
}

func workspaceProjectGrantCreate(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(w, r, tokens)
		if !ok {
			return
		}
		project, ok := authorizedProject(w, r, store, claims)
		if !ok || !projectActionAllowed(w, r, store, claims, project, authz.ProjectGrantManage) {
			return
		}
		noStore(w)
		var input struct {
			Principal *string    `json:"principal"`
			Group     *string    `json:"group"`
			Role      authz.Role `json:"role"`
		}
		if !principalDecode(w, r, &input) {
			return
		}
		err, grant := store.ProjectGrantCreate(r.Context(), project, claims.Principal.Ref, input.Principal, input.Group, input.Role)
		if err != nil {
			projectMemberError(w, r, err)
			return
		}
		writeJSONStatus(w, http.StatusCreated, grant)
	}
}

func workspaceProjectGrantUpdate(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(w, r, tokens)
		if !ok {
			return
		}
		project, ok := authorizedProject(w, r, store, claims)
		if !ok || !projectActionAllowed(w, r, store, claims, project, authz.ProjectGrantManage) {
			return
		}
		noStore(w)
		var input struct {
			Role             *authz.Role `json:"role"`
			Enabled          *bool       `json:"enabled"`
			ExpectedRevision int         `json:"expected_revision"`
		}
		if !principalDecode(w, r, &input) {
			return
		}
		err, grant := store.ProjectGrantUpdate(r.Context(), project, claims.Principal.Ref, r.PathValue("grant"), input.Role, input.Enabled, input.ExpectedRevision)
		if err != nil {
			projectMemberError(w, r, err)
			return
		}
		writeJSON(w, grant)
	}
}
