package httpservice

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/inputform"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func inputResponseHeaders(response http.ResponseWriter) {
	noStore(response)
	response.Header().Set("Referrer-Policy", "no-referrer")
}

func workspaceSessionInputOpen(store *database.Store, tokens *auth.BearerTokens, publicBaseURL string) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		inputResponseHeaders(response)
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		query := request.URL.Query()
		redirect := true
		if len(query) != 0 {
			if len(query) != 1 || len(query["redirect"]) != 1 || query.Get("redirect") != "false" {
				http.Error(response, "invalid input redirect option", http.StatusBadRequest)
				return
			}
			redirect = false
		}
		claims, ok := authenticate(response, request, tokens)
		if !ok {
			return
		}
		session, ok := authorizedSession(response, request, store, claims)
		if !ok {
			return
		}
		if !sessionActionAllowed(response, request, store, claims, session, authz.SessionInputRespond) {
			return
		}
		id := request.PathValue("input")
		if !typed_id.Valid(typed_id.SessionEvent, id) {
			http.NotFound(response, request)
			return
		}
		input := model.SessionEventRef{Session: session, Id: id}
		if _, ok := pendingInputForm(response, request, store, input); !ok {
			return
		}
		origin, err := config.CanonicalHTTPPublicBaseURL(publicBaseURL)
		if err != nil {
			http.Error(response, "input launch requires services.http.public_base_url", http.StatusServiceUnavailable)
			return
		}
		err, capability := tokens.MintFormInput(request.Context(), input, claims)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		destination := origin + "/app/input#" + url.Values{"capability": {capability}}.Encode()
		if redirect {
			response.Header().Set("Location", destination)
			response.WriteHeader(http.StatusSeeOther)
			return
		}
		writeJSON(response, struct {
			URL string `json:"url"`
		}{URL: destination})
	}
}

func pendingInputForm(response http.ResponseWriter, request *http.Request, store *database.Store, input model.SessionEventRef) (*inputform.Form, bool) {
	err, form := store.SessionInputRequestFormGet(request.Context(), input)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return nil, false
	}
	if form == nil {
		http.NotFound(response, request)
		return nil, false
	}
	if !inputPending(response, request, store, input) {
		return nil, false
	}
	return form, true
}

func inputPending(response http.ResponseWriter, request *http.Request, store *database.Store, input model.SessionEventRef) bool {
	err, resolved := store.SessionInputResolvedGet(request.Context(), input)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return false
	}
	if resolved {
		http.Error(response, "input is already resolved", http.StatusConflict)
		return false
	}
	return true
}

func authorizedFormInput(response http.ResponseWriter, request *http.Request, store *database.Store, tokens *auth.BearerTokens) (*inputform.Form, model.SessionEventRef, auth.Claims, bool) {
	err, capability, claims := tokens.AuthenticateFormInput(request.Context(), request.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthenticated) {
			response.Header().Set("WWW-Authenticate", "Bearer")
			response.WriteHeader(http.StatusUnauthorized)
		} else {
			http.Error(response, "internal server error", http.StatusInternalServerError)
		}
		return nil, model.SessionEventRef{}, auth.Claims{}, false
	}
	input := capability.Request()
	err, form := store.SessionInputRequestFormGet(request.Context(), input)
	if err != nil {
		http.Error(response, "internal server error", http.StatusInternalServerError)
		return nil, model.SessionEventRef{}, auth.Claims{}, false
	}
	if form == nil {
		http.NotFound(response, request)
		return nil, model.SessionEventRef{}, auth.Claims{}, false
	}
	if !sessionActionAllowed(response, request, store, claims, input.Session, authz.SessionInputRespond) {
		return nil, model.SessionEventRef{}, auth.Claims{}, false
	}
	return form, input, claims, true
}

func sessionInputRead(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		inputResponseHeaders(response)
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		form, input, _, ok := authorizedFormInput(response, request, store, tokens)
		if !ok {
			return
		}
		err, state := store.SessionInputStateGet(request.Context(), input)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if state == nil {
			http.NotFound(response, request)
			return
		}
		if state.Resolved {
			http.Error(response, "input is already resolved", http.StatusConflict)
			return
		}
		writeJSON(response, struct {
			Form      *inputform.Form `json:"form"`
			Draft     json.RawMessage `json:"draft"`
			UpdatedAt *string         `json:"updated_at"`
		}{Form: form, Draft: state.Draft, UpdatedAt: state.UpdatedAt})
	}
}

type inputDraftPatch struct {
	Op    string          `json:"op"`
	Path  []string        `json:"path"`
	Value json.RawMessage `json:"value"`
}

func sessionInputPatch(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		inputResponseHeaders(response)
		if request.Method != http.MethodPatch {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		form, input, _, ok := authorizedFormInput(response, request, store, tokens)
		if !ok {
			return
		}
		if !inputPending(response, request, store, input) {
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		var patch inputDraftPatch
		if err := decoder.Decode(&patch); err != nil {
			inputPatchDecodeError(response, err)
			return
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			inputPatchDecodeError(response, err)
			return
		}
		if (patch.Op != "set" && patch.Op != "remove") || patch.Op == "set" && len(patch.Value) == 0 || patch.Op == "remove" && len(patch.Value) != 0 {
			http.Error(response, "invalid input draft operation", http.StatusBadRequest)
			return
		}
		field, ok := form.FieldAtPath(patch.Path)
		if !ok {
			http.Error(response, "invalid input field path", http.StatusBadRequest)
			return
		}
		var err error
		var draft *database.SessionInputDraft
		if patch.Op == "set" {
			if err := inputform.ValidateDraftValue(field, patch.Value); err != nil {
				http.Error(response, err.Error(), http.StatusUnprocessableEntity)
				return
			}
			err, draft = store.SessionInputDraftSet(request.Context(), input, patch.Path, patch.Value)
		} else {
			err, draft = store.SessionInputDraftRemove(request.Context(), input, patch.Path)
		}
		switch {
		case errors.Is(err, database.ErrSessionInputResolved):
			http.Error(response, "input is already resolved", http.StatusConflict)
		case errors.Is(err, database.ErrSessionInputDraftPath):
			http.Error(response, "invalid input field path", http.StatusBadRequest)
		case err != nil:
			http.Error(response, "internal server error", http.StatusInternalServerError)
		case draft == nil:
			http.NotFound(response, request)
		default:
			response.WriteHeader(http.StatusNoContent)
		}
	}
}

func inputPatchDecodeError(response http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		http.Error(response, "input draft patch is too large", http.StatusRequestEntityTooLarge)
	} else {
		http.Error(response, "invalid input draft patch", http.StatusBadRequest)
	}
}

func sessionInputTerminal(store *database.Store, tokens *auth.BearerTokens, dispatcher ReplyDispatcher, kind string) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		inputResponseHeaders(response)
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, input, claims, ok := authorizedFormInput(response, request, store, tokens)
		if !ok {
			return
		}
		if !inputPending(response, request, store, input) {
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(response, request.Body, 1))
		if err != nil || len(body) != 0 {
			http.Error(response, "input terminal request must be empty", http.StatusBadRequest)
			return
		}
		id, err := typed_id.New(typed_id.SessionEvent)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		var payload map[string]interface{}
		if kind == "input.failure" {
			payload = map[string]interface{}{"code": "cancelled"}
		}
		event := model.SessionEvent{
			Ref: model.SessionEventRef{Session: input.Session, Id: id}, Parent: &input,
			Kind: kind, AuthorPrincipal: &claims.Principal, Payload: payload,
		}
		err, stored := store.SessionInputResponseCreate(request.Context(), event)
		switch {
		case errors.Is(err, database.ErrSessionInputResolved):
			http.Error(response, "input is already resolved", http.StatusConflict)
		case errors.Is(err, database.ErrSessionInputInvalidDraft):
			http.Error(response, err.Error(), http.StatusUnprocessableEntity)
		case err != nil:
			http.Error(response, "internal server error", http.StatusInternalServerError)
		default:
			if dispatcher != nil {
				_ = dispatcher.Reconcile()
			}
			writeJSONStatus(response, http.StatusAccepted, struct {
				EventID string `json:"event_id"`
				Kind    string `json:"kind"`
			}{EventID: stored.Ref.Id, Kind: stored.Kind})
		}
	}
}
