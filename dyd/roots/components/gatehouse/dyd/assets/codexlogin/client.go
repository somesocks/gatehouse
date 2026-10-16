// Package codexlogin implements the stateless ChatGPT device-code login flow.
// Credentials are exchanged and validated in memory. HTTP handlers install the
// bundle server-side and return only the public PollResult to the browser.
package codexlogin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	issuer         = "https://auth.openai.com"
	clientID       = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultPollMS  = 5000
	maximumBody    = 1 << 20
	deviceLifetime = 15 * time.Minute
)

type Client struct {
	http *http.Client
	now  func() time.Time
}

// NewClient uses bounded, non-redirecting requests by default. A supplied client
// allows transport injection for tests without configurable authentication URLs.
func NewClient(client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	}
	return &Client{http: client, now: time.Now}
}

type StartResult struct {
	DeviceAuthID    string    `json:"device_auth_id"`
	UserCode        string    `json:"user_code"`
	VerificationURI string    `json:"verification_uri"`
	ExpiresAt       time.Time `json:"expires_at"`
	PollAfterMS     int64     `json:"poll_after_ms"`
}

type Account struct {
	ID    string `json:"id"`
	Email string `json:"email,omitempty"`
}

type PollResult struct {
	Status      string     `json:"status"`
	PollAfterMS int64      `json:"poll_after_ms,omitempty"`
	Account     *Account   `json:"account,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

type Credential struct {
	Version      int       `json:"version"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at,omitzero"`
	AccountID    string    `json:"account_id"`
	Email        string    `json:"email,omitempty"`
}

func (credential Credential) Validate() error {
	if credential.Version != 1 || strings.TrimSpace(credential.AccessToken) == "" || strings.TrimSpace(credential.RefreshToken) == "" || strings.TrimSpace(credential.AccountID) == "" {
		return invalidResponse()
	}
	return nil
}

type PollOutcome struct {
	PollResult
	Credential *Credential `json:"-"`
}

// Error contains only an allowlisted diagnostic, never an upstream body or token.
type Error struct {
	Status  int
	Message string
}

func (err *Error) Error() string { return err.Message }

func (client *Client) Start(ctx context.Context) (StartResult, error) {
	var result struct {
		DeviceAuthID string      `json:"device_auth_id"`
		UserCode     string      `json:"user_code"`
		Usercode     string      `json:"usercode"`
		Interval     json.Number `json:"interval"`
	}
	started := client.now().UTC()
	_, err := client.post(ctx, "/api/accounts/deviceauth/usercode", "application/json", []byte(`{"client_id":"`+clientID+`"}`), &result, false)
	if err != nil {
		return StartResult{}, err
	}
	if result.UserCode == "" {
		result.UserCode = result.Usercode
	}
	if !validValue(result.DeviceAuthID, 4096) || !validValue(result.UserCode, 128) {
		return StartResult{}, invalidResponse()
	}
	interval := int64(defaultPollMS)
	if result.Interval != "" {
		seconds, err := result.Interval.Int64()
		if err != nil || seconds < 0 || seconds > int64(deviceLifetime/time.Second) {
			return StartResult{}, invalidResponse()
		}
		interval = max(seconds, 1) * 1000
	}
	return StartResult{
		DeviceAuthID: result.DeviceAuthID, UserCode: result.UserCode,
		VerificationURI: issuer + "/codex/device", ExpiresAt: started.Add(deviceLifetime), PollAfterMS: interval,
	}, nil
}

func (client *Client) Poll(ctx context.Context, deviceAuthID, userCode string) (PollOutcome, error) {
	if !validValue(deviceAuthID, 4096) || !validValue(userCode, 128) {
		return PollOutcome{}, &Error{http.StatusBadRequest, "A device authorization ID and user code are required."}
	}
	payload, _ := json.Marshal(struct {
		DeviceAuthID string `json:"device_auth_id"`
		UserCode     string `json:"user_code"`
	}{deviceAuthID, userCode})
	var approved struct {
		Code     string `json:"authorization_code"`
		Verifier string `json:"code_verifier"`
	}
	pending, err := client.post(ctx, "/api/accounts/deviceauth/token", "application/json", payload, &approved, true)
	if err != nil {
		return PollOutcome{}, err
	}
	if pending {
		return PollOutcome{PollResult: PollResult{Status: "pending", PollAfterMS: defaultPollMS}}, nil
	}
	if !validValue(approved.Code, 16384) || !validValue(approved.Verifier, 4096) {
		return PollOutcome{}, invalidResponse()
	}
	form := url.Values{
		"grant_type": {"authorization_code"}, "client_id": {clientID},
		"code": {approved.Code}, "code_verifier": {approved.Verifier},
		"redirect_uri": {issuer + "/deviceauth/callback"},
	}
	var tokens tokenResponse
	_, err = client.post(ctx, "/oauth/token", "application/x-www-form-urlencoded", []byte(form.Encode()), &tokens, false)
	if err != nil {
		return PollOutcome{}, err
	}
	credential, err := credentialFromResponse(tokens, nil, client.now().UTC())
	if err != nil {
		return PollOutcome{}, invalidResponse()
	}
	var expires *time.Time
	if !credential.ExpiresAt.IsZero() {
		expires = &credential.ExpiresAt
	}
	return PollOutcome{
		PollResult: PollResult{Status: "succeeded", Account: &Account{ID: credential.AccountID, Email: credential.Email}, ExpiresAt: expires},
		Credential: &credential,
	}, nil
}

func validValue(value string, maximum int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= maximum
}

type claims struct {
	AccountID string      `json:"chatgpt_account_id"`
	Email     string      `json:"email"`
	Exp       json.Number `json:"exp"`
	Auth      struct {
		AccountID string `json:"chatgpt_account_id"`
	} `json:"https://api.openai.com/auth"`
	Profile struct {
		Email string `json:"email"`
	} `json:"https://api.openai.com/profile"`
	Organizations []struct {
		ID string `json:"id"`
	} `json:"organizations"`
}

func (value claims) accountID() string {
	if value.AccountID != "" {
		return value.AccountID
	}
	if value.Auth.AccountID != "" {
		return value.Auth.AccountID
	}
	if len(value.Organizations) > 0 {
		return value.Organizations[0].ID
	}
	return ""
}

func tokenClaims(token string) claims {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims{}
	}
	var value claims
	if json.Unmarshal(payload, &value) != nil {
		return claims{}
	}
	return value
}

func invalidResponse() error {
	return &Error{http.StatusBadGateway, "OpenAI returned an invalid login response. Start a new login and try again."}
}

func (client *Client) post(ctx context.Context, path, contentType string, body []byte, target any, allowPending bool) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, issuer+path, bytes.NewReader(body))
	if err != nil {
		return false, invalidResponse()
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "gatehouse")
	response, err := client.http.Do(request)
	if err != nil {
		var network net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &network) && network.Timeout()) {
			return false, &Error{http.StatusGatewayTimeout, "OpenAI login timed out. Start a new login and try again."}
		}
		return false, &Error{http.StatusBadGateway, "OpenAI login could not be reached. Start a new login and try again."}
	}
	defer response.Body.Close()
	if allowPending && (response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusNotFound) {
		return true, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := "OpenAI login failed. Start a new login and try again."
		if path == "/api/accounts/deviceauth/usercode" && response.StatusCode == http.StatusNotFound {
			message = "Device-code login is unavailable. Check your ChatGPT account or workspace device-code login settings."
		}
		return false, &Error{http.StatusBadGateway, message}
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, maximumBody+1))
	if err != nil || len(contents) > maximumBody || json.Unmarshal(contents, target) != nil {
		return false, invalidResponse()
	}
	return false, nil
}
