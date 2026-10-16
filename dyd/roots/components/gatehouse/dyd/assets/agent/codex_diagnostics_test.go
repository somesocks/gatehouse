package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"gatehouse/database"
)

func TestCodexAcceptsRealStreamsRegardlessOfContentType(t *testing.T) {
	for _, contentType := range []string{"", "application/json", "text/plain", "invalid; content-type"} {
		t.Run(contentType, func(t *testing.T) {
			client := &http.Client{Transport: codexTestTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader("\xef\xbb\xbf" + codexTestStream()))}, nil
			})}
			result, err := codexComplete(context.Background(), client, codexTestCredential(), openAIResponsesRequest{Model: "test"})
			if err != nil || len(result.Output) != 1 {
				t.Fatalf("valid stream rejected: %v", err)
			}
		})
	}
}

func TestCodexResponseDiagnosticsAreUsefulAndSanitized(t *testing.T) {
	for _, test := range []struct {
		name, contentType, body, stage, format, code, param string
		status                                              int
	}{
		{"JSON rejection with success status", "application/json", `{"error":{"code":"model_not_found","param":"model","message":"CODEX_ACCESS_SENTINEL"}}`, "response_format", "json", "model_not_found", "model", 200},
		{"JSON rejection with stream header", "text/event-stream", `{"error":{"type":"invalid_request_error","param":"text.format","message":"CODEX_REFRESH_SENTINEL"}}`, "response_format", "json", "invalid_request_error", "text.format", 200},
		{"HTTP failure", "application/json", `{"error":{"code":"unsupported_parameter","param":"max_output_tokens","message":"CODEX_REFRESH_SENTINEL"}}`, "http_response", "json", "unsupported_parameter", "max_output_tokens", 400},
		{"HTML page", "text/html; charset=utf-8", `<html>CODEX_ACCESS_SENTINEL</html>`, "response_format", "html", "", "", 200},
		{"empty body", "", "", "response_format", "empty", "", "", 204},
		{"unknown error fields", "application/json", `{"error":{"code":"CODEX_ACCESS_SENTINEL","param":"CODEX_REFRESH_SENTINEL","message":"PRIVATE_UPSTREAM_MESSAGE"}}`, "http_response", "json", "unrecognized", "unrecognized", 403},
		{"stream rejection", "text/event-stream", codexTestEvent("response.failed", `"response":{"error":{"code":"context_length_exceeded","param":"input","message":"CODEX_ACCESS_SENTINEL"}}`), "stream", "sse", "context_length_exceeded", "input", 200},
		{"top-level stream error", "text/event-stream", codexTestEvent("error", `"code":"server_error","message":"CODEX_REFRESH_SENTINEL"`), "stream", "sse", "server_error", "", 200},
		{"truncated stream", "text/event-stream", codexTestEvent("response.created", `"response":{"id":"resp"}`), "stream", "sse", "", "", 200},
		{"invalid completed decision", "text/event-stream", codexTestEvent("response.completed", `"response":{"id":"resp","output":[]}`), "completion", "sse", "", "", 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: codexTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: test.status, Header: http.Header{"Content-Type": {test.contentType}, "X-Request-Id": {"req_codex_test"}}, Body: io.NopCloser(strings.NewReader(test.body))}, nil
			})}
			_, err := codexComplete(context.Background(), client, codexTestCredential(), openAIResponsesRequest{Model: "test"})
			var failure *codexFailure
			if !errors.As(err, &failure) || calls != 1 {
				t.Fatalf("missing diagnostic or extra request: %v", err)
			}
			if failure.status != test.status || failure.stage != test.stage || failure.bodyFormat != test.format || failure.code != test.code || failure.param != test.param || failure.requestID != "req_codex_test" {
				t.Fatalf("incorrect diagnostic: %+v", failure)
			}
			var output bytes.Buffer
			codexLogFailure(context.Background(), slog.New(slog.NewJSONHandler(&output, nil)), &database.WorkspaceAgentModel{ProviderID: "apr_test", Model: "test-model"}, err, 125*time.Millisecond)
			var record map[string]any
			if json.Unmarshal(output.Bytes(), &record) != nil || record["provider_id"] != "apr_test" || record["model"] != "test-model" || record["stage"] != test.stage || record["http_status"] != float64(test.status) || record["duration_ms"] != float64(125) || record["upstream_request_id"] != "req_codex_test" {
				t.Fatalf("missing structured correlation fields: %s", output.String())
			}
			for _, secret := range []string{"CODEX_ACCESS_SENTINEL", "CODEX_REFRESH_SENTINEL", "PRIVATE_UPSTREAM_MESSAGE"} {
				if strings.Contains(err.Error(), secret) || strings.Contains(output.String(), secret) {
					t.Fatal("diagnostic leaked upstream data")
				}
			}
			if !strings.Contains(err.Error(), "HTTP ") || !strings.Contains(err.Error(), "content-type=") {
				t.Fatal("session error omitted HTTP details")
			}
		})
	}
}

func TestCodexDiagnosticsRedactReflectedHeaderCredentials(t *testing.T) {
	client := &http.Client{Transport: codexTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/CODEX_ACCESS_SENTINEL"}, "X-Request-Id": {"CODEX_REFRESH_SENTINEL"}}, Body: io.NopCloser(strings.NewReader("<html>not a stream</html>"))}, nil
	})}
	_, err := codexComplete(context.Background(), client, codexTestCredential(), openAIResponsesRequest{})
	var failure *codexFailure
	if !errors.As(err, &failure) || failure.contentType != "redacted" || failure.requestID != "redacted" || strings.Contains(err.Error(), "SENTINEL") {
		t.Fatalf("unsafe header metadata: %v", err)
	}
}

func TestCodexDiagnosticWrappingPreservesRateLimitClassification(t *testing.T) {
	client := &http.Client{Transport: codexTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Header: http.Header{"Content-Type": {"application/json"}, "Retry-After": {"2"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"rate_limit_exceeded","message":"CODEX_ACCESS_SENTINEL"}}`))}, nil
	})}
	_, err := codexComplete(context.Background(), client, codexTestCredential(), openAIResponsesRequest{})
	var retry *openAIRetryableError
	if !errors.As(err, &retry) || retry.RetryAfter() != 2*time.Second || strings.Contains(err.Error(), "SENTINEL") {
		t.Fatalf("rate-limit classification lost: %v", err)
	}
}
