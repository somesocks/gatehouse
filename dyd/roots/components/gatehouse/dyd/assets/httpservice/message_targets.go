package httpservice

import (
	"errors"
	"net/http"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/database"
	"gatehouse/model"
)

func workspaceSessionRecipients(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
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
		scope := r.URL.Query().Get("scope")
		if scope != "" && scope != "in" && scope != "out" {
			http.Error(w, "invalid recipient scope", http.StatusBadRequest)
			return
		}
		if scope == "out" && !canManage {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		name, cursor, limit, ok := catalogSearchParameters(w, r)
		if !ok {
			return
		}
		err, targets, next := store.SessionMessageTargetsSearch(r.Context(), session, claims.Principal.Ref, database.SessionParticipantSearch{
			Kind: r.URL.Query().Get("kind"), Name: name, Cursor: cursor, Limit: limit, Outside: scope == "out",
		})
		if err != nil {
			if errors.Is(err, database.ErrMessageTarget) {
				http.Error(w, err.Error(), http.StatusBadRequest)
			} else if errors.Is(err, database.ErrSessionParticipantNotFound) || errors.Is(err, database.ErrSessionParticipantForbidden) {
				sessionParticipantError(w, r, err)
			} else {
				http.Error(w, "Recipients could not be loaded.", http.StatusInternalServerError)
			}
			return
		}
		writeJSON(w, struct {
			Targets   []model.MessageTarget `json:"targets"`
			Next      string                `json:"next_cursor,omitempty"`
			CanManage bool                  `json:"can_manage_participants"`
		}{targets, next, canManage})
	}
}
