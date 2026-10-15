package httpservice

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"gatehouse/auth"
	"gatehouse/codexlogin"
	"gatehouse/database"
)

type codexLoginClient interface {
	Start(context.Context) (codexlogin.StartResult, error)
	Poll(context.Context, string, string) (codexlogin.PollResult, error)
}

func codexLoginStart(store *database.Store, tokens *auth.BearerTokens, client codexLoginClient) http.HandlerFunc {
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
		var input struct{}
		if !codexLoginDecode(w, r, &input) {
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

func codexLoginPoll(store *database.Store, tokens *auth.BearerTokens, client codexLoginClient) http.HandlerFunc {
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
			DeviceAuthID string `json:"device_auth_id"`
			UserCode     string `json:"user_code"`
		}
		if !codexLoginDecode(w, r, &input) {
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
		writeJSON(w, result)
	}
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
