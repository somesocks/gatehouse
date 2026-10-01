package httpservice

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/inputform"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func sessionInputFieldOpen(store *database.Store, tokens *auth.BearerTokens, publicBaseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		inputResponseHeaders(w)
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		form, input, claims, ok := authorizedFormInput(w, r, store, tokens)
		if !ok || !inputPending(w, r, store, input) {
			return
		}
		var path []string
		query := r.URL.Query()
		if len(query) != 1 || len(query["path"]) != 1 || json.Unmarshal([]byte(query.Get("path")), &path) != nil {
			http.Error(w, "invalid custom field path", http.StatusBadRequest)
			return
		}
		field, found := form.FieldAtPath(path)
		if !found || field.Custom == nil {
			http.Error(w, "invalid custom field path", http.StatusBadRequest)
			return
		}
		origin, err := config.CanonicalHTTPPublicBaseURL(publicBaseURL)
		if err != nil {
			http.Error(w, "input launch requires services.http.public_base_url", http.StatusServiceUnavailable)
			return
		}
		base, _ := url.Parse(origin)
		target, err := url.Parse(field.Custom.URL)
		if err != nil {
			http.Error(w, "invalid custom field URL", http.StatusBadRequest)
			return
		}
		target = base.ResolveReference(target)
		if target.Scheme != base.Scheme || target.Host != base.Host {
			http.Error(w, "custom fields must use the Gatehouse origin", http.StatusUnprocessableEntity)
			return
		}
		err, token := tokens.MintFieldInput(r.Context(), input, claims, path, field.Custom.Capabilities)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		// URL.String escapes Fragment again. Append the already URL-encoded
		// parameters so window.location.hash needs only one decoding pass.
		fragment := url.Values{"version": {"1"}, "api": {origin + "/api/v1/input/field"}, "capability": {token}}.Encode()
		writeJSON(w, struct {
			URL string `json:"url"`
		}{target.String() + "#" + fragment})
	}
}

func authorizedCustomField(w http.ResponseWriter, r *http.Request, store *database.Store, tokens *auth.BearerTokens, operation string) (inputform.Field, auth.InputCapability, auth.Claims, bool) {
	err, capability, claims := tokens.AuthenticateFieldInput(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthenticated) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return inputform.Field{}, capability, claims, false
	}
	input := capability.Request()
	err, form := store.SessionInputRequestFormGet(r.Context(), input)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return inputform.Field{}, capability, claims, false
	}
	if form == nil {
		http.NotFound(w, r)
		return inputform.Field{}, capability, claims, false
	}
	field, found := form.FieldAtPath(capability.Path)
	if !found || field.Custom == nil || !slices.Equal(field.Custom.Capabilities, capability.Capabilities) {
		http.NotFound(w, r)
		return field, capability, claims, false
	}
	if operation != "" && !slices.Contains(capability.Capabilities, operation) {
		http.Error(w, "capability does not allow this operation", http.StatusForbidden)
		return field, capability, claims, false
	}
	if !sessionActionAllowed(w, r, store, claims, input.Session, authz.SessionInputRespond) || !inputPending(w, r, store, input) {
		return field, capability, claims, false
	}
	return field, capability, claims, true
}

func sessionInputField(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		inputResponseHeaders(w)
		if r.Method != http.MethodGet && r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		field, capability, _, ok := authorizedCustomField(w, r, store, tokens, "")
		if !ok {
			return
		}
		input := capability.Request()
		if r.Method == http.MethodGet {
			err, state := store.SessionInputStateGet(r.Context(), input)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if state == nil {
				http.NotFound(w, r)
				return
			}
			if state.Resolved {
				http.Error(w, "input is already resolved", http.StatusConflict)
				return
			}
			value := state.Draft
			for _, key := range capability.Path {
				var object map[string]json.RawMessage
				if len(value) == 0 {
					break
				}
				if json.Unmarshal(value, &object) != nil {
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}
				value = object[key]
			}
			writeJSON(w, struct {
				Label        string          `json:"label"`
				Inputs       json.RawMessage `json:"inputs"`
				Value        json.RawMessage `json:"value,omitempty"`
				Capabilities []string        `json:"capabilities"`
			}{field.Label, field.Custom.Inputs, value, field.Custom.Capabilities})
			return
		}
		var patch struct {
			Op    string          `json:"op"`
			Value json.RawMessage `json:"value"`
		}
		if !customInputBody(w, r, &patch) {
			return
		}
		if patch.Op != "set" && patch.Op != "remove" || patch.Op == "set" && !json.Valid(patch.Value) || patch.Op == "remove" && patch.Value != nil {
			http.Error(w, "invalid field operation", http.StatusBadRequest)
			return
		}
		var err error
		var draft *database.SessionInputDraft
		if patch.Op == "set" {
			err, draft = store.SessionInputDraftSet(r.Context(), input, capability.Path, patch.Value)
		} else {
			err, draft = store.SessionInputDraftRemove(r.Context(), input, capability.Path)
		}
		switch {
		case errors.Is(err, database.ErrSessionInputResolved):
			http.Error(w, "input is already resolved", http.StatusConflict)
		case errors.Is(err, database.ErrSessionInputDraftPath):
			http.Error(w, "invalid field path", http.StatusBadRequest)
		case err != nil:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		case draft == nil:
			http.NotFound(w, r)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func customInputBody(w http.ResponseWriter, r *http.Request, value any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var trailing any
	if err := decoder.Decode(value); err != nil {
		inputPatchDecodeError(w, err)
		return false
	}
	if err := decoder.Decode(&trailing); err != io.EOF {
		http.Error(w, "expected one JSON object", http.StatusBadRequest)
		return false
	}
	return true
}

func sessionInputFieldFiles(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		inputResponseHeaders(w)
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		operation := inputform.SessionFileList
		if r.Method == http.MethodPost {
			operation = inputform.SessionFileUpload
		}
		_, capability, claims, ok := authorizedCustomField(w, r, store, tokens, operation)
		if !ok {
			return
		}
		session := capability.Request().Session
		if r.Method == http.MethodGet {
			query := r.URL.Query()
			if ids, ok := query["id"]; ok {
				if len(query) != 1 || len(ids) == 0 || len(ids) > 100 {
					http.Error(w, "invalid selected file query", http.StatusBadRequest)
					return
				}
				err, files := store.SessionFilesByIDs(r.Context(), session, ids)
				if errors.Is(err, database.ErrSessionFileSearch) {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				if err != nil {
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}
				writeJSON(w, struct {
					Files []database.SessionFileSummary `json:"files"`
				}{files})
				return
			}
			search := database.SessionFileSearch{Name: query.Get("name"), MediaType: query.Get("media_type"), Accept: query["accept"], Cursor: query.Get("cursor"), Direction: query.Get("direction"), Limit: 25}
			if search.Direction == "" {
				search.Direction = "next"
			}
			for name, values := range query {
				if name != "name" && name != "media_type" && name != "accept" && name != "cursor" && name != "direction" && name != "limit" || name != "accept" && len(values) != 1 {
					http.Error(w, "invalid file search query", http.StatusBadRequest)
					return
				}
			}
			if query.Has("limit") {
				limit, err := strconv.Atoi(query.Get("limit"))
				if err != nil {
					http.Error(w, "invalid page size", http.StatusBadRequest)
					return
				}
				search.Limit = limit
			}
			err, files := store.SessionFilesSearch(r.Context(), session, search)
			if errors.Is(err, database.ErrSessionFileSearch) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, files)
			return
		}
		if !sessionActionAllowed(w, r, store, claims, session, authz.SessionFileCreate) {
			return
		}
		var body sessionFileCreateRequest
		if !customInputBody(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.Name) == "" || body.MediaType != nil && (!inputform.ValidMediaTypePattern(*body.MediaType) || strings.Contains(*body.MediaType, "*")) {
			http.Error(w, "invalid session file", http.StatusBadRequest)
			return
		}
		fileID, err := typed_id.New(typed_id.SessionFile)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		objectID, err := typed_id.New(typed_id.StorageObject)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		err, file, object := store.SessionFileCreate(r.Context(), model.SessionFile{Ref: model.SessionFileRef{Session: session, Id: fileID}, Name: body.Name, MediaType: body.MediaType, Enabled: true}, objectID, claims.Principal.Ref)
		if err != nil {
			if strings.Contains(err.Error(), "no available storage provider") {
				http.Error(w, "no storage provider available", http.StatusServiceUnavailable)
			} else {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
			return
		}
		err, token := storageToken(r.Context(), tokens, object, "put", 15*time.Minute)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSONStatus(w, http.StatusCreated, sessionFileCreateResponse{File: file, UploadURL: storageURL(r, token)})
	}
}

func sessionInputFieldFileFinish(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		inputResponseHeaders(w)
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, capability, claims, ok := authorizedCustomField(w, r, store, tokens, inputform.SessionFileUpload)
		if !ok || !sessionActionAllowed(w, r, store, claims, capability.Request().Session, authz.SessionFileFinish) {
			return
		}
		if !typed_id.Valid(typed_id.SessionFile, r.PathValue("file")) {
			http.NotFound(w, r)
			return
		}
		if body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1)); err != nil || len(body) != 0 {
			http.Error(w, "finish request must be empty", http.StatusBadRequest)
			return
		}
		err, file, object := store.SessionFileGet(r.Context(), model.SessionFileRef{Session: capability.Request().Session, Id: r.PathValue("file")}, claims.Principal.Ref)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if file == nil || object == nil {
			http.NotFound(w, r)
			return
		}
		if err := tokens.StorageClient().Finish(r.Context(), object.ID); err != nil {
			http.Error(w, "storage object is not ready", http.StatusConflict)
			return
		}
		err, file, object = store.SessionFileFinish(r.Context(), file.Ref, claims.Principal.Ref)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if file == nil || object == nil {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, inputform.FileSummary{ID: file.Ref.Id, Name: file.Name, Size: object.Size, MediaType: file.MediaType})
	}
}

func sessionInputFieldFileDownload(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		inputResponseHeaders(w)
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, capability, claims, ok := authorizedCustomField(w, r, store, tokens, inputform.SessionFileRead)
		if !ok {
			return
		}
		if !typed_id.Valid(typed_id.SessionFile, r.PathValue("file")) {
			http.NotFound(w, r)
			return
		}
		err, file, object := store.SessionFileGet(r.Context(), model.SessionFileRef{Session: capability.Request().Session, Id: r.PathValue("file")}, claims.Principal.Ref)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if file == nil || object == nil {
			http.NotFound(w, r)
			return
		}
		if object.State != "success" {
			http.Error(w, "storage object is not ready", http.StatusConflict)
			return
		}
		err, token := storageToken(r.Context(), tokens, object.ID, "get", 5*time.Minute)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, struct {
			URL string `json:"url"`
		}{storageURL(r, token)})
	}
}
