package httpservice

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/database"
	"gatehouse/inputform"
	"gatehouse/model"
	"gatehouse/typed_id"
	"strings"
	"time"
)

func inputResponseHeaders(response http.ResponseWriter) {
	noStore(response)
	response.Header().Set("Referrer-Policy", "no-referrer")
}

func workspaceSessionInputOpen(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		inputResponseHeaders(response)
		if request.Method != http.MethodGet {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if request.URL.RawQuery != "" {
			http.Error(response, "input open does not accept query parameters", http.StatusBadRequest)
			return
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
		err, capability := tokens.MintFormInput(request.Context(), input, claims)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(response, struct {
			Capability string `json:"capability"`
		}{Capability: capability})
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
			if fileErr, canonical := store.CanonicalizeInputFiles(request.Context(), input.Session, field, patch.Value); fileErr != nil {
				if errors.Is(fileErr, database.ErrSessionInputFileUnavailable) {
					http.Error(response, fileErr.Error(), http.StatusUnprocessableEntity)
				} else {
					http.Error(response, "internal server error", http.StatusInternalServerError)
				}
				return
			} else {
				patch.Value = canonical
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

type inputFileCreateRequest struct {
	Path      []string `json:"path"`
	Name      string   `json:"name"`
	MediaType *string  `json:"media_type"`
}

func sessionInputFileCreate(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		inputResponseHeaders(response)
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		form, input, claims, ok := authorizedFormInput(response, request, store, tokens)
		if !ok || !inputPending(response, request, store, input) {
			return
		}
		if !sessionActionAllowed(response, request, store, claims, input.Session, authz.SessionFileCreate) {
			return
		}
		var body inputFileCreateRequest
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		var trailing any
		if err := decoder.Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" || decoder.Decode(&trailing) != io.EOF {
			http.Error(response, "invalid input file", http.StatusBadRequest)
			return
		}
		field, found := form.FileFieldAtPath(body.Path)
		if !found || body.MediaType != nil && (!inputform.ValidMediaTypePattern(*body.MediaType) || strings.HasSuffix(*body.MediaType, "/*")) || !inputform.AllowsMediaType(field, mediaTypeValueForInput(body.MediaType)) {
			http.Error(response, "invalid input file type or field", http.StatusUnprocessableEntity)
			return
		}
		fileID, idErr := typed_id.New(typed_id.SessionFile)
		objectID, objectErr := typed_id.New(typed_id.StorageObject)
		if idErr != nil || objectErr != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		file := model.SessionFile{Ref: model.SessionFileRef{Session: input.Session, Id: fileID}, Name: body.Name, MediaType: body.MediaType, Enabled: true}
		err, stored, object := store.SessionFileCreate(request.Context(), file, objectID, claims.Principal.Ref)
		if err != nil {
			if strings.Contains(err.Error(), "no available storage provider") {
				http.Error(response, "no storage provider available", http.StatusServiceUnavailable)
			} else {
				http.Error(response, "internal server error", http.StatusInternalServerError)
			}
			return
		}
		err, token := storageToken(request.Context(), tokens, object, "put", 15*time.Minute)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSONStatus(response, http.StatusCreated, sessionFileCreateResponse{File: stored, UploadURL: storageURL(request, token)})
	}
}

func mediaTypeValueForInput(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func sessionInputFileFinish(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		inputResponseHeaders(response)
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		form, input, claims, ok := authorizedFormInput(response, request, store, tokens)
		if !ok || !inputPending(response, request, store, input) {
			return
		}
		if !sessionActionAllowed(response, request, store, claims, input.Session, authz.SessionFileFinish) {
			return
		}
		var body struct {
			Path []string `json:"path"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
		decoder.DisallowUnknownFields()
		var trailing any
		if err := decoder.Decode(&body); err != nil || decoder.Decode(&trailing) != io.EOF {
			http.Error(response, "invalid file field", http.StatusBadRequest)
			return
		}
		field, found := form.FileFieldAtPath(body.Path)
		if !found {
			http.Error(response, "invalid file field", http.StatusBadRequest)
			return
		}
		fileID := request.PathValue("file")
		if !typed_id.Valid(typed_id.SessionFile, fileID) {
			http.NotFound(response, request)
			return
		}
		err, file, object := store.SessionFileGet(request.Context(), model.SessionFileRef{Session: input.Session, Id: fileID}, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if file == nil || object == nil {
			http.NotFound(response, request)
			return
		}
		if !inputform.AllowsMediaType(field, mediaTypeValueForInput(file.MediaType)) {
			http.Error(response, "invalid file media type", http.StatusUnprocessableEntity)
			return
		}
		if err := tokens.StorageClient().Finish(request.Context(), object.ID); err != nil {
			http.Error(response, "storage object is not ready", http.StatusConflict)
			return
		}
		err, finished, completed := store.SessionFileFinish(request.Context(), file.Ref, claims.Principal.Ref)
		if err != nil {
			http.Error(response, "internal server error", http.StatusInternalServerError)
			return
		}
		if finished == nil || completed == nil {
			http.NotFound(response, request)
			return
		}
		writeJSON(response, inputform.FileSummary{ID: finished.Ref.Id, Name: finished.Name, Size: completed.Size, MediaType: finished.MediaType})
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
		if kind == model.SessionEventKindInputFailure {
			var payloadErr error
			payload, payloadErr = database.SessionEventPayloadFrom(model.InputFailurePayload{Code: "cancelled"})
			if payloadErr != nil {
				http.Error(response, "internal server error", http.StatusInternalServerError)
				return
			}
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
