package codexlogin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func oldCredential() Credential {
	return Credential{Version: 1, AccessToken: "OLD_ACCESS", RefreshToken: "OLD+REFRESH/SECRET", AccountID: "account", Email: "old@example.com", ExpiresAt: time.Date(2026, 10, 15, 11, 0, 0, 0, time.UTC)}
}

func TestRefreshPreservesOmittedRefreshTokenAndAccount(t *testing.T) {
	calls := 0
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/oauth/token" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatal("incorrect refresh endpoint/encoding")
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if len(r.Form) != 3 || r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("client_id") != clientID || r.Form.Get("refresh_token") != oldCredential().RefreshToken {
			t.Fatal("incorrect refresh grant")
		}
		if deadline, ok := r.Context().Deadline(); !ok || time.Until(deadline) > 31*time.Second {
			t.Fatal("refresh has no bounded timeout")
		}
		io.WriteString(w, `{"access_token":"NEW_ACCESS","expires_in":864000}`)
	})
	got, err := client.Refresh(context.Background(), oldCredential())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || got.AccessToken != "NEW_ACCESS" || got.RefreshToken != oldCredential().RefreshToken || got.AccountID != "account" || got.Email != "old@example.com" || !got.ExpiresAt.Equal(client.now().Add(10*24*time.Hour)) {
		t.Fatal("incorrect merged refresh credential")
	}
}

func TestRefreshRotatesAndHonorsEarlierJWTExpiry(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": jwt(`{"chatgpt_account_id":"account","exp":1792069200}`), "refresh_token": "ROTATED_REFRESH", "id_token": jwt(`{"chatgpt_account_id":"account","email":"new@example.com"}`), "expires_in": 7200})
	})
	got, err := client.Refresh(context.Background(), oldCredential())
	if err != nil || got.RefreshToken != "ROTATED_REFRESH" || got.Email != "new@example.com" || !got.ExpiresAt.Equal(client.now().Add(time.Hour)) {
		t.Fatalf("refresh metadata invalid: %v", err)
	}
}

func TestRefreshUnknownExpiryDoesNotKeepOldOrInventNewExpiry(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"access_token":"OPAQUE_NEW_ACCESS"}`)
	})
	got, err := client.Refresh(context.Background(), oldCredential())
	if err != nil || !got.ExpiresAt.IsZero() {
		t.Fatalf("expiry should be unknown: %v", err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "expires_at") {
		t.Fatal("unknown expiry was serialized as a date")
	}
}

func TestRefreshRejectionsAreClassifiedAndSanitized(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		body      string
		permanent bool
	}{
		{"expired", 400, `{"error":{"code":"refresh_token_expired","message":"SECRET"}}`, true},
		{"reused", 401, `{"error":{"code":"refresh_token_reused"}}`, true},
		{"revoked", 403, `{"error":"refresh_token_invalidated"}`, true},
		{"invalid grant", 400, `{"error":"invalid_grant"}`, true},
		{"unknown unauthorized", 401, `SECRET`, true},
		{"forbidden", 403, `SECRET`, false},
		{"rate limited", 429, `{"error":"invalid_grant","secret":"SECRET"}`, false},
		{"server error", 500, `{"error":"invalid_grant"}`, false},
		{"malformed", 200, `SECRET`, false},
		{"missing access", 200, `{"refresh_token":"SECRET"}`, false},
		{"expired new token", 200, `{"access_token":"` + jwt(`{"exp":1}`) + `"}`, false},
		{"different account", 200, `{"access_token":"SECRET","id_token":"` + jwt(`{"chatgpt_account_id":"other"}`) + `"}`, true},
		{"conflicting account claims", 200, `{"access_token":"` + jwt(`{"chatgpt_account_id":"other"}`) + `","id_token":"` + jwt(`{"chatgpt_account_id":"account"}`) + `"}`, true},
		{"oversized", 200, strings.Repeat("SECRET", maximumBody), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(test.status)
				io.WriteString(w, test.body)
			})
			_, err := client.Refresh(context.Background(), oldCredential())
			var failure *RefreshError
			if !errors.As(err, &failure) || failure.Permanent != test.permanent || calls != 1 {
				t.Fatalf("incorrect refresh failure: %v", err)
			}
			if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), oldCredential().RefreshToken) {
				t.Fatal("refresh error leaked credentials")
			}
		})
	}
}

func TestRefreshCancellationDoesNotBlindlyRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	client := NewClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) { calls++; return nil, r.Context().Err() })})
	_, err := client.Refresh(ctx, oldCredential())
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("cancellation = %v calls=%d", err, calls)
	}
}
