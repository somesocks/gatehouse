package codexlogin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RefreshError never includes token endpoint bodies, URLs, or credential values.
type RefreshError struct{ Permanent bool }

func (err *RefreshError) Error() string {
	if err.Permanent {
		return "ChatGPT authorization is no longer valid; reconnect the provider"
	}
	return "ChatGPT credentials could not be refreshed"
}

type tokenResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	IDToken      string      `json:"id_token"`
	ExpiresIn    json.Number `json:"expires_in"`
}

var errAccountChanged = errors.New("refreshed ChatGPT account changed")

func (client *Client) Refresh(ctx context.Context, previous Credential) (Credential, error) {
	if previous.Validate() != nil {
		return Credential{}, &RefreshError{Permanent: true}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	form := url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "refresh_token": {previous.RefreshToken}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, issuer+"/oauth/token", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return Credential{}, &RefreshError{}
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "gatehouse")
	response, err := client.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return Credential{}, ctx.Err()
		}
		return Credential{}, &RefreshError{}
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, maximumBody+1))
	if err != nil || len(contents) > maximumBody {
		return Credential{}, &RefreshError{}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		permanent := response.StatusCode == http.StatusUnauthorized
		if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusForbidden {
			permanent = permanentRefreshCode(contents)
		}
		return Credential{}, &RefreshError{Permanent: permanent}
	}
	var tokens tokenResponse
	if json.Unmarshal(contents, &tokens) != nil {
		return Credential{}, &RefreshError{}
	}
	credential, err := credentialFromResponse(tokens, &previous, client.now().UTC())
	if err != nil {
		return Credential{}, &RefreshError{Permanent: errors.Is(err, errAccountChanged)}
	}
	return credential, nil
}

func permanentRefreshCode(contents []byte) bool {
	var payload struct {
		Error json.RawMessage `json:"error"`
		Code  string          `json:"code"`
	}
	if json.Unmarshal(contents, &payload) != nil {
		return false
	}
	code := payload.Code
	var text string
	if json.Unmarshal(payload.Error, &text) == nil {
		code = text
	} else {
		var object struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(payload.Error, &object) == nil && object.Code != "" {
			code = object.Code
		}
	}
	switch strings.ToLower(code) {
	case "invalid_grant", "refresh_token_expired", "refresh_token_reused", "refresh_token_invalidated":
		return true
	}
	return false
}

func credentialFromResponse(tokens tokenResponse, previous *Credential, now time.Time) (Credential, error) {
	credential := Credential{Version: 1, AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken}
	if previous != nil {
		credential.AccountID, credential.Email = previous.AccountID, previous.Email
		if credential.RefreshToken == "" {
			credential.RefreshToken = previous.RefreshToken
		}
	}
	// Claims are obtained only from the fixed HTTPS token endpoint. They are
	// upstream account metadata, never Gatehouse authentication claims.
	id, access := tokenClaims(tokens.IDToken), tokenClaims(tokens.AccessToken)
	for _, candidate := range []string{id.accountID(), access.accountID()} {
		if candidate == "" {
			continue
		}
		if credential.AccountID != "" && credential.AccountID != candidate {
			return Credential{}, errAccountChanged
		}
		credential.AccountID = candidate
	}
	for _, email := range []string{id.Email, access.Email, access.Profile.Email} {
		if email != "" {
			credential.Email = email
			break
		}
	}
	if tokens.ExpiresIn != "" {
		seconds, err := tokens.ExpiresIn.Int64()
		if err != nil || seconds <= 0 || seconds > int64(365*24*time.Hour/time.Second) {
			return Credential{}, invalidResponse()
		}
		credential.ExpiresAt = now.Add(time.Duration(seconds) * time.Second)
	}
	if access.Exp != "" {
		seconds, err := access.Exp.Int64()
		if err != nil || seconds <= 0 {
			return Credential{}, invalidResponse()
		}
		expires := time.Unix(seconds, 0).UTC()
		if credential.ExpiresAt.IsZero() || expires.Before(credential.ExpiresAt) {
			credential.ExpiresAt = expires
		}
	}
	if !credential.ExpiresAt.IsZero() && !credential.ExpiresAt.After(now) {
		return Credential{}, invalidResponse()
	}
	if err := credential.Validate(); err != nil {
		return Credential{}, err
	}
	return credential, nil
}
