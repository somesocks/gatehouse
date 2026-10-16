package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gatehouse/codexlogin"
	"gatehouse/database"
)

// Only sanitized metadata and local error messages belong in this type. Error()
// may be persisted in a session event or a DBOS failure, not just a server log.
type codexFailure struct {
	cause       error
	stage       string
	status      int
	contentType string
	requestID   string
	bodyFormat  string
	code        string
	param       string
}

func (failure *codexFailure) Error() string {
	details := []string{"stage=" + failure.stage}
	if failure.status != 0 {
		details = append(details, fmt.Sprintf("HTTP %d", failure.status), "content-type="+fmt.Sprintf("%q", failure.contentType))
	}
	if failure.bodyFormat != "" {
		details = append(details, "body="+failure.bodyFormat)
	}
	if failure.code != "" {
		details = append(details, "code="+failure.code)
	}
	if failure.param != "" {
		details = append(details, "param="+failure.param)
	}
	return failure.cause.Error() + " (" + strings.Join(details, ", ") + ")"
}

func (failure *codexFailure) Unwrap() error { return failure.cause }

func codexLogFailure(ctx context.Context, logger *slog.Logger, selected *database.WorkspaceAgentModel, err error, elapsed time.Duration) {
	fields := []any{"provider_id", selected.ProviderID, "model", selected.Model, "agent_id", selected.Ref.Id,
		"duration_ms", elapsed.Milliseconds(), "error", err.Error()}
	var failure *codexFailure
	if errors.As(err, &failure) {
		fields = append(fields, "stage", failure.stage, "http_status", failure.status, "content_type", failure.contentType,
			"body_format", failure.bodyFormat, "upstream_request_id", failure.requestID, "upstream_code", failure.code, "upstream_param", failure.param)
	} else {
		fields = append(fields, "stage", "credentials")
	}
	logger.WarnContext(ctx, "Codex inference failed", fields...)
}

// Header values are bounded, printable, and checked against both credentials.
// In particular a malicious/misconfigured peer must not reflect a token through
// Content-Type or X-Request-ID after we have sanitized its response body.
func codexSafeHeader(value string, credential codexlogin.Credential) string {
	if len(value) > 256 {
		return "redacted"
	}
	for _, char := range value {
		if char < 32 || char > 126 {
			return "redacted"
		}
	}
	for _, secret := range []string{credential.AccessToken, credential.RefreshToken} {
		if secret != "" && strings.Contains(strings.ToLower(value), strings.ToLower(secret)) {
			return "redacted"
		}
	}
	return value
}

// Look at a bounded prefix without waiting for a whole SSE event. JSON and HTML
// can be reported accurately even when their HTTP Content-Type is misleading.
func codexBodyFormat(reader *bufio.Reader) string {
	for i := 0; i < 4096; i++ {
		prefix, err := reader.Peek(1)
		if err != nil {
			return "empty"
		}
		switch prefix[0] {
		case 0xef:
			bom, _ := reader.Peek(3)
			if string(bom) != "\xef\xbb\xbf" {
				return "unknown"
			}
			reader.Discard(3)
		case ' ', '\t', '\r', '\n':
			reader.ReadByte()
		case '{', '[':
			return "json"
		case '<':
			return "html"
		case ':', 'd', 'e', 'i', 'r': // SSE comments, data, event, id, retry
			return "sse"
		default:
			return "unknown"
		}
	}
	return "unknown"
}

func codexUpstreamFailure(cause error, payload []byte) *codexFailure {
	var envelope struct {
		Error json.RawMessage `json:"error"`
		Code  string          `json:"code"`
		Type  string          `json:"type"`
		Param string          `json:"param"`
	}
	failure := &codexFailure{cause: cause}
	if json.Unmarshal(payload, &envelope) != nil {
		return failure
	}
	code, param := envelope.Code, envelope.Param
	if code == "" {
		code = envelope.Type
	}
	if len(envelope.Error) != 0 {
		var object struct {
			Code  string `json:"code"`
			Type  string `json:"type"`
			Param string `json:"param"`
		}
		var text string
		if json.Unmarshal(envelope.Error, &object) == nil {
			if object.Code != "" {
				code = object.Code
			} else if object.Type != "" {
				code = object.Type
			}
			if object.Param != "" {
				param = object.Param
			}
		} else if json.Unmarshal(envelope.Error, &text) == nil {
			code = text
		}
	}
	// Never copy arbitrary error messages, codes, or parameter strings. These
	// finite vocabularies are request/API metadata, not upstream free-form text.
	switch code {
	case "model_not_found", "model_not_supported", "unsupported_model", "invalid_model", "invalid_request_error",
		"unsupported_parameter", "unsupported_value", "invalid_value", "missing_required_parameter", "unknown_parameter",
		"context_length_exceeded", "rate_limit_exceeded", "usage_limit_reached", "insufficient_quota",
		"invalid_api_key", "authentication_error", "permission_denied", "server_error", "content_filter":
		failure.code = code
	default:
		if code != "" {
			failure.code = "unrecognized"
		}
	}
	switch param {
	case "model", "instructions", "input", "tools", "tool_choice", "parallel_tool_calls", "stream", "store",
		"max_output_tokens", "max_tokens", "temperature", "top_p", "reasoning", "reasoning.effort", "reasoning.summary",
		"text", "text.format", "text.format.type", "text.format.schema", "include", "previous_response_id":
		failure.param = param
	default:
		if param != "" {
			failure.param = "unrecognized"
		}
	}
	return failure
}
