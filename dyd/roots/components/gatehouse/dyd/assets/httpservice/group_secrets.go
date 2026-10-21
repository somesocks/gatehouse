package httpservice

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/database"
	"gatehouse/model"
	"gatehouse/typed_id"
)

type groupSecretRequest struct {
	Description      *string `json:"description"`
	Value            *string `json:"value"`
	ExpectedRevision int     `json:"expected_revision"`
}

func groupSecretDecode(w http.ResponseWriter, r *http.Request, input any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		http.Error(w, "invalid group secret", http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		http.Error(w, "invalid group secret", http.StatusBadRequest)
		return false
	}
	return true
}

type groupSecretResponse struct {
	ID          string                    `json:"id"`
	Group       string                    `json:"group"`
	Description string                    `json:"description"`
	Author      projectNoteAuthorResponse `json:"author"`
	Revision    int                       `json:"revision"`
	CreatedAt   string                    `json:"created_at"`
	UpdatedAt   string                    `json:"updated_at"`
}

func groupSecretResponseFromDetail(detail database.GroupSecretDetail) groupSecretResponse {
	s := detail.Secret
	return groupSecretResponse{ID: s.Ref.Id, Group: s.Ref.Group.Id, Description: s.Description, Author: projectNoteAuthorResponse{ID: s.AuthorPrincipal.Id, Name: detail.AuthorName}, Revision: s.Revision, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
}

func groupSecretError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, database.ErrGroupSecretInput):
		http.Error(w, "invalid group secret", http.StatusBadRequest)
	case errors.Is(err, database.ErrGroupSecretNotFound), errors.Is(err, database.ErrGroupNotFound):
		http.NotFound(w, r)
	case errors.Is(err, database.ErrGroupForbidden):
		http.Error(w, "group manager access is required", http.StatusForbidden)
	case errors.Is(err, database.ErrRevisionConflict):
		http.Error(w, "This secret changed elsewhere. Reload before retrying.", http.StatusConflict)
	default:
		http.Error(w, "Group secrets could not be updated.", http.StatusInternalServerError)
	}
}

func validGroupSecretRequest(input groupSecretRequest, create bool) bool {
	if create {
		if input.Description == nil || input.Value == nil || input.ExpectedRevision != 0 {
			return false
		}
	} else if input.ExpectedRevision < 1 || (input.Description == nil && input.Value == nil) {
		return false
	}
	return (input.Description == nil || (strings.TrimSpace(*input.Description) != "" && len(*input.Description) <= 4096)) &&
		(input.Value == nil || (*input.Value != "" && len(*input.Value) <= 1024*1024))
}

func workspaceGroupSecrets(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		noStore(w)
		actor, ok := groupRequestScope(w, r, store, tokens, false, r.Method == http.MethodPost)
		if !ok {
			return
		}
		workspace := model.WorkspaceRef{Id: r.PathValue("workspace")}
		group := model.GroupRef{Id: r.PathValue("group")}
		if r.Method == http.MethodGet {
			name, cursor, limit, ok := catalogSearchParameters(w, r)
			if !ok {
				return
			}
			if cursor != "" && !typed_id.Valid(typed_id.GroupSecret, cursor) {
				http.Error(w, "invalid secret cursor", http.StatusBadRequest)
				return
			}
			err, secrets := store.GroupSecretsGet(r.Context(), workspace, group, *actor, database.CollectionPage{Name: name, Cursor: cursor, Limit: limit})
			if err != nil {
				groupSecretError(w, r, err)
				return
			}
			next := ""
			if len(secrets) > limit {
				secrets, next = secrets[:limit], secrets[limit-1].Ref.Id
			}
			items := make([]groupSecretResponse, 0, len(secrets))
			for _, s := range secrets {
				items = append(items, groupSecretResponse{ID: s.Ref.Id, Group: s.Ref.Group.Id, Description: s.Description, Author: projectNoteAuthorResponse{ID: s.AuthorPrincipal.Id, Name: s.AuthorName}, Revision: s.Revision, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt})
			}
			err, role := store.GroupRoleGet(r.Context(), workspace.Id, group.Id, *actor)
			if err != nil {
				groupSecretError(w, r, err)
				return
			}
			writeJSON(w, struct {
				Secrets    []groupSecretResponse `json:"secrets"`
				NextCursor string                `json:"next_cursor,omitempty"`
				CanManage  bool                  `json:"can_manage_secrets"`
			}{items, next, authz.GroupAllows([]authz.Role{role}, authz.GroupSecretEdit)})
			return
		}
		var input groupSecretRequest
		if !groupSecretDecode(w, r, &input) {
			return
		}
		if !validGroupSecretRequest(input, true) {
			groupSecretError(w, r, database.ErrGroupSecretInput)
			return
		}
		id, err := typed_id.New(typed_id.GroupSecret)
		if err != nil {
			groupSecretError(w, r, err)
			return
		}
		ref := model.GroupSecretRef{Workspace: workspace, Group: group, Id: id}
		value := []byte(*input.Value)
		err, ciphertext := tokens.Encrypt(r.Context(), database.GroupSecretAssociatedData(ref), value)
		clear(value)
		if err != nil {
			groupSecretError(w, r, err)
			return
		}
		err, secret := store.GroupSecretCreate(r.Context(), model.GroupSecret{Ref: ref, Description: *input.Description, Ciphertext: ciphertext}, *actor)
		if err != nil {
			groupSecretError(w, r, err)
			return
		}
		writeJSONStatus(w, http.StatusCreated, groupSecretResponseFromDetail(database.GroupSecretDetail{Secret: secret}))
	}
}

func workspaceGroupSecret(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPatch && r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		noStore(w)
		actor, ok := groupRequestScope(w, r, store, tokens, false, r.Method != http.MethodGet)
		if !ok {
			return
		}
		ref := model.GroupSecretRef{Workspace: model.WorkspaceRef{Id: r.PathValue("workspace")}, Group: model.GroupRef{Id: r.PathValue("group")}, Id: r.PathValue("secret")}
		if !typed_id.Valid(typed_id.GroupSecret, ref.Id) {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			err, detail := store.GroupSecretGet(r.Context(), ref, *actor)
			if err != nil {
				groupSecretError(w, r, err)
				return
			}
			writeJSON(w, groupSecretResponseFromDetail(*detail))
			return
		}
		if r.Method == http.MethodDelete {
			var input struct {
				ExpectedRevision int `json:"expected_revision"`
			}
			if !groupSecretDecode(w, r, &input) {
				return
			}
			if err := store.GroupSecretRemove(r.Context(), ref, *actor, input.ExpectedRevision); err != nil {
				groupSecretError(w, r, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var input groupSecretRequest
		if !groupSecretDecode(w, r, &input) {
			return
		}
		if !validGroupSecretRequest(input, false) {
			groupSecretError(w, r, database.ErrGroupSecretInput)
			return
		}
		var ciphertext *string
		if input.Value != nil {
			value := []byte(*input.Value)
			err, encrypted := tokens.Encrypt(r.Context(), database.GroupSecretAssociatedData(ref), value)
			clear(value)
			if err != nil {
				groupSecretError(w, r, err)
				return
			}
			ciphertext = &encrypted
		}
		err, detail := store.GroupSecretUpdate(r.Context(), ref, *actor, input.Description, ciphertext, input.ExpectedRevision)
		if err != nil {
			groupSecretError(w, r, err)
			return
		}
		writeJSON(w, groupSecretResponseFromDetail(*detail))
	}
}
