package httpservice

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/database"
	"gatehouse/typed_id"
)

func systemPrincipals(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok || !systemActionAllowed(response, request, store, claims, authz.SystemManage) {
			return
		}
		if request.Method == http.MethodPost {
			var input struct {
				Alias   *string `json:"alias"`
				Name    *string `json:"name"`
				Enabled *bool   `json:"enabled"`
			}
			if !principalDecode(response, request, &input) {
				return
			}
			err, principal := store.SystemPrincipalCreate(request.Context(), database.SystemPrincipalCreateInput{Alias: input.Alias, Name: input.Name, Enabled: input.Enabled == nil || *input.Enabled})
			principalResult(response, request, err, principal)
			return
		}
		if request.URL.Query().Has("limit") {
			systemPrincipalsSearch(response, request, store)
			return
		}
		err, principals := store.SystemPrincipalsGet(request.Context())
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(response, principals)
	}
}

func systemPrincipalIdentities(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !systemManage(w, r, store, tokens) {
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var input struct {
			Username *string `json:"username"`
			Password *string `json:"password"`
			Enabled  *bool   `json:"enabled"`
		}
		if !principalDecode(w, r, &input) {
			return
		}
		err, principal := store.SystemPrincipalIdentityCreate(r.Context(), r.PathValue("principal"), database.SystemPrincipalIdentityInput{Username: input.Username, Password: input.Password, Enabled: input.Enabled})
		principalResult(w, r, err, principal)
	}
}

func systemPrincipalIdentity(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !systemManage(w, r, store, tokens) {
			return
		}
		if r.Method != http.MethodPatch && r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Method == http.MethodDelete {
			var input struct {
				ExpectedRevision *int `json:"expected_revision"`
			}
			if !principalDecode(w, r, &input) {
				return
			}
			if input.ExpectedRevision == nil {
				http.Error(w, "expected revision is required", http.StatusBadRequest)
				return
			}
			err, principal := store.SystemPrincipalIdentityRemove(r.Context(), r.PathValue("principal"), r.PathValue("identity"), *input.ExpectedRevision)
			principalResult(w, r, err, principal)
			return
		}
		var input struct {
			Username         *string `json:"username"`
			Password         *string `json:"password"`
			Enabled          *bool   `json:"enabled"`
			ExpectedRevision *int    `json:"expected_revision"`
		}
		if !principalDecode(w, r, &input) {
			return
		}
		if input.ExpectedRevision == nil {
			http.Error(w, "expected revision is required", http.StatusBadRequest)
			return
		}
		err, principal := store.SystemPrincipalIdentityUpdate(r.Context(), r.PathValue("principal"), r.PathValue("identity"), *input.ExpectedRevision, database.SystemPrincipalIdentityInput{Username: input.Username, Password: input.Password, Enabled: input.Enabled})
		principalResult(w, r, err, principal)
	}
}

func principalDecode(w http.ResponseWriter, r *http.Request, target any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		http.Error(w, "invalid principal administration request", http.StatusBadRequest)
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		http.Error(w, "invalid principal administration request", http.StatusBadRequest)
		return false
	}
	return true
}

func principalResult(w http.ResponseWriter, r *http.Request, err error, principal *database.SystemPrincipal) {
	if err != nil {
		switch {
		case errors.Is(err, database.ErrRevisionConflict), errors.Is(err, database.ErrPrincipalAliasExists), errors.Is(err, database.ErrIdentityUsernameExists):
			http.Error(w, err.Error(), http.StatusConflict)
		case errors.Is(err, database.ErrPrincipalInput):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}
	if principal == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, principal)
}

func systemPrincipal(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodPatch {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok || !systemActionAllowed(response, request, store, claims, authz.SystemManage) {
			return
		}
		id := request.PathValue("principal")
		if !typed_id.Valid(typed_id.Principal, id) {
			http.NotFound(response, request)
			return
		}
		if request.Method == http.MethodGet {
			err, principal := store.SystemPrincipalGet(request.Context(), id)
			if err != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
			if principal == nil {
				http.NotFound(response, request)
				return
			}
			writeJSON(response, principal)
			return
		}
		var input systemPrincipalUpdateRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || input.Enabled == nil {
			http.Error(response, "invalid system principal update", http.StatusBadRequest)
			return
		}
		err, principal := store.SystemPrincipalEnabledSet(request.Context(), id, *input.Enabled)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if principal == nil {
			http.NotFound(response, request)
			return
		}
		writeJSON(response, principal)
	}
}
