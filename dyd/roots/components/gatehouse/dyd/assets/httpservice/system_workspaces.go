package httpservice

import (
	"errors"
	"net/http"

	"gatehouse/auth"
	"gatehouse/database"
	"gatehouse/typed_id"
)

type systemWorkspaceSearchResponse struct {
	Workspaces []database.SystemWorkspace `json:"workspaces"`
	NextCursor string                     `json:"next_cursor,omitempty"`
}

type systemWorkspaceCreateRequest struct {
	Alias   string  `json:"alias"`
	Name    *string `json:"name"`
	Enabled *bool   `json:"enabled"`
}

type systemWorkspaceUpdateRequest struct {
	Name    *string `json:"name"`
	Enabled *bool   `json:"enabled"`
}

func systemWorkspaces(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !systemManage(response, request, store, tokens) {
			return
		}
		if request.Method == http.MethodGet {
			name, cursor, limit, ok := catalogSearchParameters(response, request)
			if !ok {
				return
			}
			if cursor != "" && !typed_id.Valid(typed_id.Workspace, cursor) {
				http.Error(response, "invalid workspace cursor", http.StatusBadRequest)
				return
			}
			err, workspaces, nextCursor := store.SystemWorkspacesSearch(request.Context(), database.SystemWorkspaceSearch{
				Name:   name,
				Cursor: cursor,
				Limit:  limit,
			})
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			writeJSON(response, systemWorkspaceSearchResponse{Workspaces: workspaces, NextCursor: nextCursor})
			return
		}

		var input systemWorkspaceCreateRequest
		if !adminDecode(response, request, &input) {
			return
		}
		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		err, workspace := store.SystemWorkspaceCreate(request.Context(), input.Alias, input.Name, enabled)
		if errors.Is(err, database.ErrWorkspaceAliasExists) {
			http.Error(response, err.Error(), http.StatusConflict)
			return
		}
		if errors.Is(err, database.ErrWorkspaceAliasInvalid) || errors.Is(err, database.ErrWorkspaceNameInvalid) {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(response, "workspace could not be created", http.StatusInternalServerError)
			return
		}
		writeJSONStatus(response, http.StatusCreated, workspace)
	}
}

func systemWorkspace(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPatch {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !systemManage(response, request, store, tokens) {
			return
		}
		id := request.PathValue("workspace")
		if request.Method == http.MethodGet {
			err, workspace := store.SystemWorkspaceGet(request.Context(), id)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			if workspace == nil {
				http.NotFound(response, request)
				return
			}
			writeJSON(response, workspace)
			return
		}

		var input systemWorkspaceUpdateRequest
		if !adminDecode(response, request, &input) {
			return
		}
		if input.Name == nil && input.Enabled == nil {
			http.Error(response, "workspace update is empty", http.StatusBadRequest)
			return
		}
		err, workspace := store.SystemWorkspaceUpdate(request.Context(), id, input.Name, input.Enabled)
		if err != nil {
			http.Error(response, "workspace could not be updated", http.StatusInternalServerError)
			return
		}
		if workspace == nil {
			http.NotFound(response, request)
			return
		}
		writeJSON(response, workspace)
	}
}
