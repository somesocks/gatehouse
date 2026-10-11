package httpservice

import (
	"errors"
	"net/http"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/database"
	"gatehouse/typed_id"
)

type systemWorkspacePrincipalPage struct {
	Principals []database.SystemWorkspacePrincipal `json:"principals"`
	NextCursor string                              `json:"next_cursor,omitempty"`
}

type systemWorkspacePrincipalOptionPage struct {
	Principals []database.SystemWorkspacePrincipalOption `json:"principals"`
	NextCursor string                                    `json:"next_cursor,omitempty"`
}

type systemWorkspacePrincipalCreateRequest struct {
	Principal string `json:"principal"`
	Role      string `json:"role"`
}

func systemWorkspacePrincipals(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !systemManage(response, request, store, tokens) {
			return
		}
		workspaceID := request.PathValue("workspace")
		err, workspace := store.SystemWorkspaceGet(request.Context(), workspaceID)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if workspace == nil {
			http.NotFound(response, request)
			return
		}

		if request.Method == http.MethodGet {
			name, cursor, limit, ok := catalogSearchParameters(response, request)
			if !ok {
				return
			}
			if cursor != "" && !typed_id.Valid(typed_id.Principal, cursor) {
				http.Error(response, "invalid principal cursor", http.StatusBadRequest)
				return
			}
			err, principals, nextCursor := store.SystemWorkspacePrincipalsSearch(request.Context(), database.SystemWorkspacePrincipalSearch{
				WorkspaceID: workspaceID,
				Name:        name,
				Cursor:      cursor,
				Limit:       limit,
			})
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			writeJSON(response, systemWorkspacePrincipalPage{Principals: principals, NextCursor: nextCursor})
			return
		}

		var input systemWorkspacePrincipalCreateRequest
		if !adminDecode(response, request, &input) {
			return
		}
		if !typed_id.Valid(typed_id.Principal, input.Principal) {
			http.Error(response, "invalid principal", http.StatusBadRequest)
			return
		}
		if !authz.ValidRole(input.Role) {
			http.Error(response, "invalid workspace role", http.StatusBadRequest)
			return
		}
		err, grant := store.SystemWorkspacePrincipalGrantCreate(request.Context(), workspaceID, input.Principal, authz.Role(input.Role))
		if errors.Is(err, database.ErrWorkspaceGrantAlreadyExists) {
			http.Error(response, err.Error(), http.StatusConflict)
			return
		}
		if errors.Is(err, database.ErrWorkspaceGrantPrincipalNotFound) {
			http.NotFound(response, request)
			return
		}
		if err != nil {
			http.Error(response, "workspace principal could not be added", http.StatusInternalServerError)
			return
		}
		if grant == nil {
			http.NotFound(response, request)
			return
		}
		writeJSONStatus(response, http.StatusCreated, grant)
	}
}

func systemWorkspacePrincipalOptions(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !systemManage(response, request, store, tokens) {
			return
		}
		workspaceID := request.PathValue("workspace")
		err, workspace := store.SystemWorkspaceGet(request.Context(), workspaceID)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if workspace == nil {
			http.NotFound(response, request)
			return
		}
		name, cursor, limit, ok := catalogSearchParameters(response, request)
		if !ok {
			return
		}
		if cursor != "" && !typed_id.Valid(typed_id.Principal, cursor) {
			http.Error(response, "invalid principal cursor", http.StatusBadRequest)
			return
		}
		err, principals, nextCursor := store.SystemWorkspacePrincipalOptionsSearch(request.Context(), database.SystemWorkspacePrincipalSearch{
			WorkspaceID: workspaceID,
			Name:        name,
			Cursor:      cursor,
			Limit:       limit,
		})
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(response, systemWorkspacePrincipalOptionPage{Principals: principals, NextCursor: nextCursor})
	}
}

func systemWorkspacePrincipalRemove(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !systemManage(response, request, store, tokens) {
			return
		}
		err, removed := store.SystemWorkspacePrincipalGrantsRemove(request.Context(), request.PathValue("workspace"), request.PathValue("principal"))
		if err != nil {
			http.Error(response, "workspace principal could not be removed", http.StatusInternalServerError)
			return
		}
		if !removed {
			http.NotFound(response, request)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	}
}
