package httpservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gatehouse/auth"
	"gatehouse/codexlogin"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
)

type codexLoginClient interface {
	Start(context.Context) (codexlogin.StartResult, error)
	Poll(context.Context, string, string) (codexlogin.PollOutcome, error)
}

type codexProviderRequest struct {
	Provider         string `json:"provider"`
	ExpectedRevision int    `json:"expected_revision"`
}

func codexLoginStart(store *database.Store, ring *keychain.Keyring, tokens *auth.BearerTokens, client codexLoginClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !systemManage(w, r, store, tokens) {
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var input codexProviderRequest
		if !codexLoginDecode(w, r, &input) {
			return
		}
		if codexLoginProvider(w, r, store, ring, input) == nil {
			return
		}
		result, err := client.Start(r.Context())
		if err != nil {
			codexLoginError(w, err)
			return
		}
		writeJSON(w, result)
	}
}

func codexLoginPoll(store *database.Store, ring *keychain.Keyring, tokens *auth.BearerTokens, client codexLoginClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !systemManage(w, r, store, tokens) {
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var input struct {
			codexProviderRequest
			DeviceAuthID string `json:"device_auth_id"`
			UserCode     string `json:"user_code"`
		}
		if !codexLoginDecode(w, r, &input) {
			return
		}
		provider := codexLoginProvider(w, r, store, ring, input.codexProviderRequest)
		if provider == nil {
			return
		}
		if !codexLoginValue(input.DeviceAuthID, 4096) || !codexLoginValue(input.UserCode, 128) {
			http.Error(w, "A device authorization ID and user code are required.", http.StatusBadRequest)
			return
		}
		result, err := client.Poll(r.Context(), input.DeviceAuthID, input.UserCode)
		if err != nil {
			codexLoginError(w, err)
			return
		}
		if result.Status == "pending" {
			writeJSON(w, result.PollResult)
			return
		}
		if result.Status != "succeeded" || result.Credential == nil || result.Account == nil || result.ExpiresAt == nil {
			codexLoginError(w, fmt.Errorf("missing exchanged credential"))
			return
		}
		if err := result.Credential.Validate(); err != nil {
			codexLoginError(w, err)
			return
		}
		if !systemManage(w, r, store, tokens) {
			return
		}
		plain, err := json.Marshal(result.Credential)
		if err != nil {
			codexCredentialError(w, err)
			return
		}
		defer clear(plain)
		text := string(plain)
		sealed, err := systemCredential(r.Context(), ring, &provider.Keychain.Id, &text, "gh=v1|agent-provider|alias="+provider.Alias)
		if err != nil {
			codexCredentialError(w, err)
			return
		}
		account := &database.SystemAgentProviderAccount{ID: result.Account.ID, Email: result.Account.Email, ExpiresAt: *result.ExpiresAt}
		err, updated := store.SystemAgentProviderCodexCredentialSet(r.Context(), provider.ID, input.ExpectedRevision, sealed.ref, sealed.text, account)
		if err != nil {
			codexCredentialError(w, err)
			return
		}
		writeJSON(w, struct {
			codexlogin.PollResult
			Provider *database.SystemAgentProvider `json:"provider"`
		}{result.PollResult, updated})
	}
}

func codexLoginProvider(w http.ResponseWriter, r *http.Request, store *database.Store, ring *keychain.Keyring, input codexProviderRequest) *database.SystemAgentProvider {
	if input.Provider == "" || input.ExpectedRevision < 1 {
		http.Error(w, "A provider and expected revision are required.", 400)
		return nil
	}
	err, provider := store.SystemAgentProviderGet(r.Context(), input.Provider)
	if err != nil {
		codexCredentialError(w, err)
		return nil
	}
	if provider == nil {
		http.NotFound(w, r)
		return nil
	}
	if provider.Revision != input.ExpectedRevision {
		codexCredentialError(w, database.ErrRevisionConflict)
		return nil
	}
	if provider.Protocol != config.AgentProviderProtocolOpenAICodex || provider.Keychain == nil {
		http.Error(w, "This provider does not use ChatGPT login.", 400)
		return nil
	}
	if ring == nil {
		http.Error(w, "Provider keychain is unavailable.", 503)
		return nil
	}
	if _, err := systemCredential(r.Context(), ring, &provider.Keychain.Id, nil, ""); err != nil {
		http.Error(w, "Provider keychain is unavailable.", 503)
		return nil
	}
	return provider
}

func codexProviderDisconnect(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !systemManage(w, r, store, tokens) {
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			w.WriteHeader(405)
			return
		}
		var input struct {
			ExpectedRevision int `json:"expected_revision"`
		}
		if !codexLoginDecode(w, r, &input) {
			return
		}
		if input.ExpectedRevision < 1 {
			http.Error(w, "Expected revision is required.", 400)
			return
		}
		err, provider := store.SystemAgentProviderCodexCredentialSet(r.Context(), r.PathValue("provider"), input.ExpectedRevision, nil, nil, nil)
		if err != nil {
			codexCredentialError(w, err)
			return
		}
		writeJSON(w, provider)
	}
}

func codexCredentialError(w http.ResponseWriter, err error) {
	if errors.Is(err, database.ErrRevisionConflict) {
		http.Error(w, "This provider changed. Reload its status before trying again.", 409)
		return
	}
	if errors.Is(err, database.ErrAdminDependencyMissing) {
		http.Error(w, "Provider not found.", 404)
		return
	}
	http.Error(w, "Provider credentials could not be saved.", 500)
}

func codexLoginDecode(w http.ResponseWriter, r *http.Request, value any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "Invalid Codex login request.", http.StatusBadRequest)
		return false
	}
	return true
}

func codexLoginValue(value string, maximum int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= maximum
}

func codexLoginError(w http.ResponseWriter, err error) {
	var upstream *codexlogin.Error
	if errors.As(err, &upstream) {
		http.Error(w, upstream.Message, upstream.Status)
		return
	}
	http.Error(w, "OpenAI login failed. Start a new login and try again.", http.StatusBadGateway)
}
