package httpservice

import (
	"errors"
	"net/http"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/database"
)

func sessionParticipantError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, database.ErrSessionParticipantInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, database.ErrSessionParticipantNotFound):
		http.NotFound(w, r)
	case errors.Is(err, database.ErrSessionParticipantForbidden):
		http.Error(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, database.ErrSessionGrantExists), errors.Is(err, database.ErrSessionLastManager):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, database.ErrRevisionConflict):
		http.Error(w, "This participant changed elsewhere. Reload before retrying.", http.StatusConflict)
	default:
		http.Error(w, "Chat participants could not be updated.", http.StatusInternalServerError)
	}
}

func workspaceSessionParticipants(store *database.Store, tokens *auth.BearerTokens, optionsKind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(w, r, tokens)
		if !ok {
			return
		}
		session, ok := authorizedSession(w, r, store, claims)
		if !ok {
			return
		}
		err, roles := store.SessionRolesGet(r.Context(), session, claims.Principal.Ref)
		if err != nil {
			sessionParticipantError(w, r, err)
			return
		}
		canManage := authz.SessionAllows(roles, authz.SessionGrantManage)
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
		err, items, next := store.SessionParticipantsSearch(r.Context(), session, claims.Principal.Ref, database.SessionParticipantSearch{Kind: kind, Name: name, Cursor: cursor, Limit: limit}, optionsKind != "")
		if err != nil {
			sessionParticipantError(w, r, err)
			return
		}
		writeJSON(w, struct {
			Participants []database.SessionParticipant `json:"participants"`
			NextCursor   string                        `json:"next_cursor,omitempty"`
			CanManage    bool                          `json:"can_manage_participants"`
		}{items, next, canManage})
	}
}

func workspaceSessionGrantCreate(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(w, r, tokens)
		if !ok {
			return
		}
		session, ok := authorizedSession(w, r, store, claims)
		if !ok || !sessionActionAllowed(w, r, store, claims, session, authz.SessionGrantManage) {
			return
		}
		var input struct {
			Principal *string    `json:"principal"`
			Group     *string    `json:"group"`
			Agent     *string    `json:"agent"`
			Role      authz.Role `json:"role"`
		}
		if !principalDecode(w, r, &input) {
			return
		}
		err, grant := store.SessionParticipantGrantCreateSubject(r.Context(), session, claims.Principal.Ref, input.Principal, input.Group, input.Agent, input.Role)
		if err != nil {
			sessionParticipantError(w, r, err)
			return
		}
		writeJSONStatus(w, http.StatusCreated, grant)
	}
}

func workspaceSessionGrantUpdate(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claims, ok := authenticate(w, r, tokens)
		if !ok {
			return
		}
		session, ok := authorizedSession(w, r, store, claims)
		if !ok || !sessionActionAllowed(w, r, store, claims, session, authz.SessionGrantManage) {
			return
		}
		var input struct {
			Role             *authz.Role `json:"role"`
			Enabled          *bool       `json:"enabled"`
			ExpectedRevision int         `json:"expected_revision"`
		}
		if !principalDecode(w, r, &input) {
			return
		}
		err, grant := store.SessionParticipantGrantUpdate(r.Context(), session, claims.Principal.Ref, r.PathValue("grant"), input.Role, input.Enabled, input.ExpectedRevision)
		if err != nil {
			sessionParticipantError(w, r, err)
			return
		}
		writeJSON(w, grant)
	}
}
