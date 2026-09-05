package httpservice

import (
	"encoding/json"
	"net/http"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/database"
	"gatehouse/typed_id"
)

func systemPrincipals(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet { response.WriteHeader(http.StatusMethodNotAllowed); return }
		claims, ok := authenticate(response, request, tokens)
		if !ok || !systemActionAllowed(response, request, store, claims, authz.SystemManage) { return }
		err, principals := store.SystemPrincipalsGet(request.Context())
		if err != nil { http.Error(response, "internal server error", http.StatusInternalServerError); return }
		writeJSON(response, principals)
	}
}

func systemPrincipal(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPatch { response.WriteHeader(http.StatusMethodNotAllowed); return }
		claims, ok := authenticate(response, request, tokens)
		if !ok || !systemActionAllowed(response, request, store, claims, authz.SystemManage) { return }
		id := request.PathValue("principal")
		if !typed_id.Valid(typed_id.Principal, id) { http.NotFound(response, request); return }
		if request.Method == http.MethodGet {
			err, principal := store.SystemPrincipalGet(request.Context(), id)
			if err != nil { http.Error(response, "internal server error", http.StatusInternalServerError); return }
			if principal == nil { http.NotFound(response, request); return }
			writeJSON(response, principal)
			return
		}
		var input systemPrincipalUpdateRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || input.Enabled == nil { http.Error(response, "invalid system principal update", http.StatusBadRequest); return }
		err, principal := store.SystemPrincipalEnabledSet(request.Context(), id, *input.Enabled)
		if err != nil { http.Error(response, "internal server error", http.StatusInternalServerError); return }
		if principal == nil { http.NotFound(response, request); return }
		writeJSON(response, principal)
	}
}
