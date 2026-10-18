package httpservice

import (
	"errors"
	"net/http"

	"gatehouse/auth"
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
		name, cursor, limit, ok := catalogSearchParameters(w, r)
		if !ok {
			return
		}
		err, targets, next := store.SessionMessageTargetsSearch(r.Context(), session, database.SessionParticipantSearch{
			Kind: r.URL.Query().Get("kind"), Name: name, Cursor: cursor, Limit: limit,
		})
		if err != nil {
			if errors.Is(err, database.ErrMessageTarget) {
				http.Error(w, err.Error(), http.StatusBadRequest)
			} else {
				http.Error(w, "Recipients could not be loaded.", http.StatusInternalServerError)
			}
			return
		}
		writeJSON(w, struct {
			Targets []model.MessageTarget `json:"targets"`
			Next    string                `json:"next_cursor,omitempty"`
		}{targets, next})
	}
}
