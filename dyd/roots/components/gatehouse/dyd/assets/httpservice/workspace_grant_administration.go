package httpservice

import (
	"errors"
	"net/http"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/database"
)

func systemWorkspaceGrant(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !systemManage(w, r, store, tokens) {
			return
		}
		var input struct {
			Role             *string `json:"role"`
			Enabled          *bool   `json:"enabled"`
			ExpectedRevision *int    `json:"expected_revision"`
		}
		if !principalDecode(w, r, &input) {
			return
		}
		if (input.Role != nil && !authz.ValidRole(*input.Role)) || (input.Role == nil && input.Enabled == nil) {
			http.Error(w, "invalid workspace role", http.StatusBadRequest)
			return
		}
		if input.ExpectedRevision == nil || *input.ExpectedRevision < 1 {
			http.Error(w, "a positive expected revision is required", http.StatusBadRequest)
			return
		}
		var role *authz.Role
		if input.Role != nil {
			value := authz.Role(*input.Role)
			role = &value
		}
		err, grant := store.SystemWorkspaceGrantUpdate(r.Context(), r.PathValue("workspace"), r.PathValue("grant"), role, input.Enabled, *input.ExpectedRevision)
		switch {
		case errors.Is(err, database.ErrWorkspaceGrantAlreadyExists):
			http.Error(w, "Another grant for this recipient already has that role, including disabled grants.", http.StatusConflict)
		case errors.Is(err, database.ErrRevisionConflict):
			http.Error(w, "This grant changed elsewhere. Reload it before retrying.", http.StatusConflict)
		case err != nil:
			http.Error(w, "Workspace grant could not be updated.", http.StatusInternalServerError)
		case grant == nil:
			http.NotFound(w, r)
		default:
			writeJSON(w, grant)
		}
	}
}
