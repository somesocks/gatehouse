package httpservice

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gatehouse/auth"
	"gatehouse/config"
	"gatehouse/database"
)

func systemIdentityPasswordLink(store *database.Store, tokens *auth.BearerTokens, publicBaseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		inputResponseHeaders(w)
		if !systemManage(w, r, store, tokens) {
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var input struct {
			ExpectedRevision *int `json:"expected_revision"`
		}
		if !principalDecode(w, r, &input) {
			return
		}
		if input.ExpectedRevision == nil || *input.ExpectedRevision < 0 {
			http.Error(w, "expected revision is required", http.StatusBadRequest)
			return
		}
		err, login := store.PasswordSetIdentityGet(r.Context(), r.PathValue("principal"), r.PathValue("identity"))
		if err != nil {
			passwordSetError(w, err)
			return
		}
		if login == nil {
			http.NotFound(w, r)
			return
		}
		if login.Revision != *input.ExpectedRevision {
			passwordSetError(w, database.ErrRevisionConflict)
			return
		}
		origin, err := config.CanonicalHTTPPublicBaseURL(publicBaseURL)
		if err != nil {
			http.Error(w, "password links require services.http.public_base_url", http.StatusServiceUnavailable)
			return
		}
		err, encoded, capability := tokens.MintPasswordSet(r.Context(), r.PathValue("principal"), login.ID, login.Revision)
		if err != nil {
			passwordSetError(w, err)
			return
		}
		writeJSON(w, struct {
			URL       string `json:"url"`
			ExpiresAt string `json:"expires_at"`
		}{origin + "/app/set-password/#" + url.Values{"capability": {encoded}}.Encode(), time.Unix(capability.ExpiresAt, 0).UTC().Format(time.RFC3339)})
	}
}

func passwordSet(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		inputResponseHeaders(w)
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.RawQuery != "" {
			http.Error(w, "set-password does not accept query parameters", http.StatusBadRequest)
			return
		}
		// Deliberately do not use requestAuthorization: a login cookie is not
		// a password-setting capability, even for the identity's owner.
		err, capability := tokens.AuthenticatePasswordSet(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			passwordSetError(w, err)
			return
		}
		err, login := store.PasswordSetIdentityGet(r.Context(), capability.Principal, capability.Identity)
		if err != nil {
			passwordSetError(w, err)
			return
		}
		if login == nil {
			passwordSetError(w, database.ErrPasswordLinkUnavailable)
			return
		}
		if login.Revision != capability.IdentityRevision {
			passwordSetError(w, database.ErrRevisionConflict)
			return
		}
		if r.Method == http.MethodGet {
			writeJSON(w, struct {
				Username  string `json:"username"`
				ExpiresAt string `json:"expires_at"`
			}{strings.TrimPrefix(login.Key, "gatehouse:"), time.Unix(capability.ExpiresAt, 0).UTC().Format(time.RFC3339)})
			return
		}
		var input struct {
			Password string `json:"password"`
		}
		if !principalDecode(w, r, &input) {
			return
		}
		password := []byte(input.Password)
		input.Password = ""
		defer clear(password)
		if err := store.PasswordSet(r.Context(), capability.Principal, capability.Identity, capability.IdentityRevision, time.Unix(capability.ExpiresAt, 0), password); err != nil {
			passwordSetError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func passwordSetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrPasswordLinkExpired), errors.Is(err, database.ErrPasswordLinkExpired):
		http.Error(w, "This link has expired. Ask an administrator for a new link.", http.StatusGone)
	case errors.Is(err, database.ErrRevisionConflict):
		http.Error(w, "This login has changed or the link has already been used. Ask an administrator for a new link.", http.StatusConflict)
	case errors.Is(err, auth.ErrUnauthenticated), errors.Is(err, database.ErrPasswordLinkUnavailable):
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "This link is invalid or no longer available. Ask an administrator for a new link.", http.StatusUnauthorized)
	case errors.Is(err, database.ErrPrincipalInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
