package httpservice

import (
	"encoding/json"
	"errors"
	"net/http"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func systemGrants(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok || !systemActionAllowed(response, request, store, claims, authz.SystemManage) { return }
		if request.Method == http.MethodGet {
			err, grants := store.SystemGrantsGet(request.Context())
			if err != nil { http.Error(response, "internal server error", http.StatusInternalServerError); return }
			writeJSON(response, grants)
			return
		}
		var input systemGrantCreateRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || !typed_id.Valid(typed_id.Principal, input.Principal) {
			http.Error(response, "invalid system grant", http.StatusBadRequest)
			return
		}
		err, grant := store.SystemGrantCreate(request.Context(), model.PrincipalRef{Id: input.Principal})
		if errors.Is(err, database.ErrSystemGrantPrincipalNotFound) { http.NotFound(response, request); return }
		if errors.Is(err, database.ErrSystemGrantAlreadyExists) { http.Error(response, "system grant already exists", http.StatusConflict); return }
		if err != nil { http.Error(response, "internal server error", http.StatusInternalServerError); return }
		writeJSONStatus(response, http.StatusCreated, grant)
	}
}

func systemGrant(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPatch {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok || !systemActionAllowed(response, request, store, claims, authz.SystemManage) { return }
		id := request.PathValue("grant")
		if !typed_id.Valid(typed_id.SystemGrant, id) { http.NotFound(response, request); return }
		if request.Method == http.MethodGet {
			err, grant := store.SystemGrantGet(request.Context(), id)
			if err != nil { http.Error(response, "internal server error", http.StatusInternalServerError); return }
			if grant == nil { http.NotFound(response, request); return }
			writeJSON(response, grant)
			return
		}
		var input systemGrantUpdateRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || input.Enabled == nil { http.Error(response, "invalid system grant update", http.StatusBadRequest); return }
		err, grant := store.SystemGrantEnabledSet(request.Context(), id, *input.Enabled)
		if err != nil { http.Error(response, "internal server error", http.StatusInternalServerError); return }
		if grant == nil { http.NotFound(response, request); return }
		writeJSON(response, grant)
	}
}

func systemActionAllowed(response http.ResponseWriter, request *http.Request, store *database.Store, claims auth.Claims, action authz.SystemAction) bool {
	err, roles := store.SystemRolesGet(request.Context(), claims.Principal.Ref)
	if err != nil { http.Error(response, "internal server error", http.StatusInternalServerError); return false }
	if !authz.SystemAllows(roles, action) { http.Error(response, "forbidden", http.StatusForbidden); return false }
	return true
}
