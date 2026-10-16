package codexlogin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (fn transportFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func testClient(t *testing.T, serve func(http.ResponseWriter, *http.Request)) *Client {
	t.Helper()
	client := NewClient(&http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Scheme != "https" || request.URL.Host != "auth.openai.com" {
			t.Fatalf("unexpected upstream request: %s %s", request.Method, request.URL)
		}
		response := httptest.NewRecorder()
		serve(response, request)
		return response.Result(), nil
	})})
	client.now = func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }
	return client
}

func jwt(payload string) string {
	return "eyJhbGciOiJSUzI1NiJ9." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".signature"
}

func TestStartDeviceLogin(t *testing.T) {
	for _, interval := range []string{`"7"`, `7`, `0`, `null`} {
		t.Run(interval, func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/accounts/deviceauth/usercode" || r.Header.Get("Content-Type") != "application/json" {
					t.Fatal("incorrect start request")
				}
				body, _ := io.ReadAll(r.Body)
				if string(body) != `{"client_id":"`+clientID+`"}` {
					t.Fatalf("start body = %s", body)
				}
				io.WriteString(w, `{"device_auth_id":"device","usercode":"ABCD-EFGH","interval":`+interval+`}`)
			})
			result, err := client.Start(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			wantInterval := int64(7000)
			if interval == "0" {
				wantInterval = 1000
			}
			if interval == "null" {
				wantInterval = defaultPollMS
			}
			if result.DeviceAuthID != "device" || result.UserCode != "ABCD-EFGH" || result.VerificationURI != issuer+"/codex/device" || result.PollAfterMS != wantInterval || !result.ExpiresAt.Equal(client.now().Add(deviceLifetime)) {
				t.Fatalf("unexpected start result: %#v", result)
			}
		})
	}
}

func TestPollPendingDoesNotExchange(t *testing.T) {
	for _, status := range []int{403, 404} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/api/accounts/deviceauth/token" {
					t.Fatal("unexpected exchange")
				}
				body, _ := io.ReadAll(r.Body)
				if string(body) != `{"device_auth_id":"device","user_code":"ABCD-EFGH"}` {
					t.Fatalf("poll body = %s", body)
				}
				w.WriteHeader(status)
			})
			result, err := client.Poll(context.Background(), "device", "ABCD-EFGH")
			if err != nil || result.Status != "pending" || result.Account != nil || calls != 1 {
				t.Fatalf("pending = %#v, %v, calls %d", result, err, calls)
			}
		})
	}
}

func TestApprovedPollExchangesAndOnlyReturnsMetadata(t *testing.T) {
	calls := 0
	access := jwt(`{"exp":1792067400}`) // 2026-10-15 12:30 UTC.
	identity := jwt(`{"https://api.openai.com/auth":{"chatgpt_account_id":"account"},"email":"user@example.com"}`)
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/api/accounts/deviceauth/token":
			io.WriteString(w, `{"authorization_code":"AUTH+CODE","code_verifier":"VERIFIER/secret"}`)
		case "/oauth/token":
			if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Fatal("token request must be form encoded")
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			for name, want := range map[string]string{"grant_type": "authorization_code", "client_id": clientID, "code": "AUTH+CODE", "code_verifier": "VERIFIER/secret", "redirect_uri": issuer + "/deviceauth/callback"} {
				if r.Form.Get(name) != want {
					t.Fatalf("token form %s = %q", name, r.Form.Get(name))
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": access, "refresh_token": "REFRESH_SECRET", "id_token": identity, "expires_in": 3600})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})
	result, err := client.Poll(context.Background(), "device", "ABCD-EFGH")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || result.Status != "succeeded" || result.Account == nil || result.Account.ID != "account" || result.Account.Email != "user@example.com" || result.ExpiresAt == nil || !result.ExpiresAt.Equal(client.now().Add(30*time.Minute)) {
		t.Fatalf("success = %#v, calls %d", result, calls)
	}
	encoded, _ := json.Marshal(result)
	for _, secret := range []string{access, identity, "REFRESH_SECRET", "AUTH+CODE", "VERIFIER/secret", "access_token", "refresh_token", "id_token"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("result exposed %s", secret)
		}
	}
}

func TestTokenValidationAndExpiryFallback(t *testing.T) {
	for _, test := range []struct {
		name, tokenJSON string
		valid           bool
	}{
		{"access claim fallback", `{"access_token":"` + jwt(`{"chatgpt_account_id":"account","exp":1792069200,"https://api.openai.com/profile":{"email":"user@example.com"}}`) + `","refresh_token":"secret"}`, true},
		{"opaque access expiry fallback", `{"access_token":"opaque","refresh_token":"secret","id_token":"` + jwt(`{"chatgpt_account_id":"account"}`) + `"}`, true},
		{"missing refresh", `{"access_token":"opaque"}`, false},
		{"missing account", `{"access_token":"opaque","refresh_token":"secret"}`, false},
		{"expired access", `{"access_token":"` + jwt(`{"chatgpt_account_id":"account","exp":1}`) + `","refresh_token":"secret"}`, false},
		{"invalid expiry", `{"access_token":"opaque","refresh_token":"secret","expires_in":-1,"id_token":"` + jwt(`{"chatgpt_account_id":"account"}`) + `"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/accounts/deviceauth/token" {
					io.WriteString(w, `{"authorization_code":"code","code_verifier":"verifier"}`)
				} else {
					io.WriteString(w, test.tokenJSON)
				}
			})
			result, err := client.Poll(context.Background(), "device", "code")
			if (err == nil) != test.valid {
				t.Fatalf("result = %#v, error = %v", result, err)
			}
			if test.valid {
				if result.Account == nil || result.Account.ID != "account" {
					t.Fatalf("metadata = %#v", result)
				}
				if test.name == "opaque access expiry fallback" {
					if result.ExpiresAt != nil || !result.Credential.ExpiresAt.IsZero() {
						t.Fatal("unknown expiry was fabricated")
					}
				} else if result.ExpiresAt == nil || !result.ExpiresAt.Equal(client.now().Add(time.Hour)) {
					t.Fatalf("metadata = %#v", result)
				}
			}
		})
	}
}

func TestUpstreamFailuresAreSanitizedAndBounded(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"upstream rejection", 500, `{"error":"TOKEN_SECRET"}`},
		{"redirect", 302, "TOKEN_SECRET"},
		{"malformed", 200, "TOKEN_SECRET"},
		{"incomplete", 200, `{}`},
		{"oversized", 200, strings.Repeat("TOKEN_SECRET", maximumBody)},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(test.status); io.WriteString(w, test.body) })
			_, err := client.Start(context.Background())
			var failure *Error
			if !errors.As(err, &failure) || failure.Status != http.StatusBadGateway || strings.Contains(err.Error(), "TOKEN_SECRET") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCancelledPollDoesNotExchange(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewClient(&http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/api/accounts/deviceauth/token" || request.Context().Err() != context.Canceled {
			t.Fatal("cancellation was not propagated")
		}
		return nil, request.Context().Err()
	})})
	if _, err := client.Poll(ctx, "device", "code"); err == nil {
		t.Fatal("cancelled poll succeeded")
	}
}
