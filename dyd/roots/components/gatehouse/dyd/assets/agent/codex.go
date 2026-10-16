package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"gatehouse/codexlogin"
	"gatehouse/config"
	"gatehouse/database"
)

const (
	codexInferenceTimeout   = 5 * time.Minute
	codexMaximumEventBytes  = 1 << 20
	codexMaximumStreamBytes = 16 << 20
)

// Codex uses Responses over SSE, but does not accept the public API's output
// token limit. Keep the actual wire body separate, including for token estimates.
type codexRequest struct {
	Model             string                    `json:"model"`
	Instructions      string                    `json:"instructions"`
	Input             []json.RawMessage         `json:"input"`
	Tools             []openAIResponsesTool     `json:"tools,omitempty"`
	Text              *openAIResponsesText      `json:"text,omitempty"`
	Reasoning         *openAIResponsesReasoning `json:"reasoning,omitempty"`
	ParallelToolCalls bool                      `json:"parallel_tool_calls"`
	ToolChoice        string                    `json:"tool_choice"`
	Store             bool                      `json:"store"`
	Stream            bool                      `json:"stream"`
}

func codexRequestFromResponses(request openAIResponsesRequest) codexRequest {
	return codexRequest{Model: request.Model, Instructions: request.Instructions, Input: request.Input,
		Tools: request.Tools, Text: request.Text, Reasoning: request.Reasoning,
		ParallelToolCalls: request.ParallelToolCalls, ToolChoice: "auto", Store: false, Stream: true}
}

func (runtime *SessionEventReplyRuntime) codexComplete(ctx context.Context, selected *database.WorkspaceAgentModel, request openAIResponsesRequest) (result openAIResponsesResponse, failure error) {
	started := time.Now()
	defer func() {
		if failure != nil {
			codexLogFailure(ctx, slog.Default(), selected, failure, time.Since(started))
		}
	}()
	if selected.Endpoint == nil || *selected.Endpoint != config.OpenAICodexEndpoint {
		return openAIResponsesResponse{}, errors.New("Codex inference requires the fixed Codex endpoint")
	}
	if selected.APIKey == nil || selected.Keychain == nil {
		return openAIResponsesResponse{}, errors.New("Codex provider is disconnected; connect ChatGPT in provider settings")
	}
	err, plain := runtime.openAIAPIKey(ctx, selected)
	if err != nil {
		return openAIResponsesResponse{}, errors.New("Codex credentials could not be decrypted")
	}
	defer clear(plain)
	var credential codexlogin.Credential
	if json.Unmarshal(plain, &credential) != nil || credential.Validate() != nil {
		return openAIResponsesResponse{}, errors.New("Codex credentials are invalid; reconnect the provider")
	}
	// No refresh, credential writes, or authorization retry on the inference path.
	return codexComplete(ctx, &http.Client{}, credential, request)
}

func codexComplete(ctx context.Context, client *http.Client, credential codexlogin.Credential, completion openAIResponsesRequest) (result openAIResponsesResponse, failure error) {
	diagnostic := codexFailure{stage: "credentials"}
	defer func() {
		if failure == nil {
			return
		}
		var detail *codexFailure
		if errors.As(failure, &detail) {
			diagnostic.code, diagnostic.param = detail.code, detail.param
			if detail.stage != "" {
				diagnostic.stage = detail.stage
			}
			failure = detail.cause
		}
		diagnostic.cause = failure
		failure = &diagnostic
	}()
	ctx, cancel := context.WithTimeout(ctx, codexInferenceTimeout)
	defer cancel()
	if credential.Validate() != nil {
		return openAIResponsesResponse{}, errors.New("Codex credentials are invalid")
	}
	diagnostic.stage = "request_encoding"
	payload, err := json.Marshal(codexRequestFromResponses(completion))
	if err != nil {
		return openAIResponsesResponse{}, errors.New("Codex request could not be encoded")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.OpenAICodexEndpoint, bytes.NewReader(payload))
	if err != nil {
		return openAIResponsesResponse{}, errors.New("Codex request could not be created")
	}
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	request.Header.Set("ChatGPT-Account-ID", credential.AccountID)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("User-Agent", "gatehouse")
	request.Header.Set("Originator", "gatehouse")
	// Reject redirects even for injected clients: credentials belong to the fixed
	// endpoint. Neither transport errors nor upstream bodies become diagnostics.
	bounded := *client
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	started := time.Now()
	diagnostic.stage = "http_request"
	response, err := bounded.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return openAIResponsesResponse{}, ctx.Err()
		}
		return openAIResponsesResponse{}, errors.New("Codex inference request failed")
	}
	defer response.Body.Close()
	diagnostic.status = response.StatusCode
	diagnostic.contentType = codexSafeHeader(response.Header.Get("Content-Type"), credential)
	if diagnostic.contentType == "" {
		diagnostic.contentType = "missing"
	}
	diagnostic.requestID = codexSafeHeader(response.Header.Get("X-Request-ID"), credential)
	diagnostic.stage = "http_response"
	reader := bufio.NewReader(response.Body)
	diagnostic.bodyFormat = codexBodyFormat(reader)
	if ctx.Err() != nil {
		return openAIResponsesResponse{}, ctx.Err()
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := "Codex inference request was rejected"
		if response.StatusCode == http.StatusUnauthorized {
			message = "Codex authorization failed"
		}
		contents, err := io.ReadAll(io.LimitReader(reader, codexMaximumEventBytes+1))
		if ctx.Err() != nil {
			return openAIResponsesResponse{}, ctx.Err()
		}
		if err == nil && len(contents) <= codexMaximumEventBytes {
			detail := codexUpstreamFailure(errors.New(message), contents)
			diagnostic.code, diagnostic.param = detail.code, detail.param
		}
		if response.StatusCode == http.StatusTooManyRequests && diagnostic.code == "rate_limit_exceeded" {
			if after, ok := openAIRetryAfter(response.Header); ok && after > 0 {
				return openAIResponsesResponse{}, &openAIRetryableError{message: message, retryAfter: after}
			}
		}
		return openAIResponsesResponse{}, errors.New(message)
	}
	diagnostic.stage = "response_format"
	if diagnostic.bodyFormat != "sse" {
		message := errors.New("Codex returned a non-stream response")
		if diagnostic.bodyFormat == "json" {
			contents, err := io.ReadAll(io.LimitReader(reader, codexMaximumEventBytes+1))
			if ctx.Err() != nil {
				return openAIResponsesResponse{}, ctx.Err()
			}
			if err == nil && len(contents) <= codexMaximumEventBytes {
				return openAIResponsesResponse{}, codexUpstreamFailure(message, contents)
			}
		}
		return openAIResponsesResponse{}, message
	}
	diagnostic.stage = "stream"
	result, err = codexReadStream(reader, started)
	if ctx.Err() != nil {
		return openAIResponsesResponse{}, ctx.Err()
	}
	if err == nil && completion.MaxOutputTokens > 0 && result.Metrics.OutputTokens != nil && *result.Metrics.OutputTokens > completion.MaxOutputTokens {
		diagnostic.stage = "completion"
		err = errors.New("Codex response exceeded the configured output token limit")
	}
	return result, err
}

// Consume complete output items, never execute deltas. A terminal completion is
// required even when all output items have arrived. Some Codex streams omit the
// terminal output array and supply it only via output_item.done events.
func codexReadStream(body io.Reader, started time.Time) (openAIResponsesResponse, error) {
	limited := &io.LimitedReader{R: body, N: codexMaximumStreamBytes + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), codexMaximumEventBytes)
	var frame []byte
	items := map[int]json.RawMessage{}
	for scanner.Scan() {
		if limited.N <= 0 {
			return openAIResponsesResponse{}, errors.New("Codex stream exceeded the size limit")
		}
		line := scanner.Bytes()
		if len(line) != 0 {
			if bytes.HasPrefix(line, []byte("data:")) {
				data := bytes.TrimPrefix(line[5:], []byte(" "))
				if len(frame)+len(data)+1 > codexMaximumEventBytes {
					return openAIResponsesResponse{}, errors.New("Codex stream event exceeded the size limit")
				}
				frame = append(frame, data...)
				frame = append(frame, '\n')
			}
			continue
		}
		if len(frame) == 0 {
			continue
		}
		var event struct {
			Type        string          `json:"type"`
			OutputIndex *int            `json:"output_index"`
			Item        json.RawMessage `json:"item"`
			Error       json.RawMessage `json:"error"`
			Code        string          `json:"code"`
			Param       string          `json:"param"`
			Response    *struct {
				ID      string                `json:"id"`
				Status  string                `json:"status"`
				Output  []json.RawMessage     `json:"output"`
				Usage   *openAIResponsesUsage `json:"usage"`
				EndTurn *bool                 `json:"end_turn"`
				Error   json.RawMessage       `json:"error"`
			} `json:"response"`
		}
		if json.Unmarshal(frame, &event) != nil || event.Type == "" {
			return openAIResponsesResponse{}, errors.New("Codex returned an invalid stream event")
		}
		frame = frame[:0]
		switch event.Type {
		case "error", "response.failed", "response.incomplete":
			result := openAIResponsesResponse{}
			if event.Response != nil {
				result.Metrics = openAIResponsesMetrics(event.Response.Usage, time.Since(started))
				if !codexMetricsValid(result) {
					result.Metrics = nil
				}
			}
			upstream := event.Error
			if event.Response != nil && len(event.Response.Error) != 0 {
				upstream = event.Response.Error
			}
			payload, _ := json.Marshal(struct {
				Error json.RawMessage `json:"error"`
				Code  string          `json:"code"`
				Param string          `json:"param"`
			}{upstream, event.Code, event.Param})
			return result, codexUpstreamFailure(errors.New("Codex inference did not complete successfully"), payload)
		case "response.output_item.done":
			index := len(items)
			if event.OutputIndex != nil {
				index = *event.OutputIndex
			}
			if index < 0 || index > 4096 || len(event.Item) == 0 || bytes.Equal(event.Item, []byte("null")) || items[index] != nil {
				return openAIResponsesResponse{}, errors.New("Codex returned an invalid output item")
			}
			items[index] = event.Item
		case "response.completed":
			if event.Response == nil || strings.TrimSpace(event.Response.ID) == "" || (event.Response.Status != "" && event.Response.Status != "completed") {
				return openAIResponsesResponse{}, &codexFailure{cause: errors.New("Codex returned an invalid completion"), stage: "completion"}
			}
			result := openAIResponsesResponse{Output: event.Response.Output, Metrics: openAIResponsesMetrics(event.Response.Usage, time.Since(started))}
			if !codexMetricsValid(result) {
				return openAIResponsesResponse{}, errors.New("Codex returned invalid token usage")
			}
			if len(result.Output) == 0 {
				indices := make([]int, 0, len(items))
				for index := range items {
					indices = append(indices, index)
				}
				sort.Ints(indices)
				for _, index := range indices {
					result.Output = append(result.Output, items[index])
				}
			}
			var err error
			result.Output, err = codexOutputsNormalize(result.Output, event.Response.EndTurn)
			if err != nil {
				return result, &codexFailure{cause: err, stage: "completion"}
			}
			return result, nil
		}
	}
	// Do not include scanner errors: an injected transport may contain secrets in
	// its error string. EOF, [DONE], and complete items alone are not completion.
	return openAIResponsesResponse{}, errors.New("Codex stream ended before a complete response")
}

func codexOutputsNormalize(outputs []json.RawMessage, endTurn *bool) ([]json.RawMessage, error) {
	usable, calls := false, false
	ids := map[string]bool{}
	normalized := []json.RawMessage{}
	for _, raw := range outputs {
		var item struct {
			openAIResponsesOutput
			Phase string `json:"phase"`
		}
		if json.Unmarshal(raw, &item) != nil || (item.Status != "" && item.Status != "completed") {
			return nil, errors.New("Codex returned an invalid output item")
		}
		switch item.Type {
		case "reasoning":
			continue
		case "message":
			if item.Phase != "" && item.Phase != "final_answer" {
				continue
			}
			item.Status = "completed"
			usable = true
		case "function_call":
			if strings.TrimSpace(item.CallID) == "" || strings.TrimSpace(item.Name) == "" || ids[item.CallID] {
				return nil, errors.New("Codex returned an invalid tool call")
			}
			ids[item.CallID] = true
			usable, calls = true, true
		default:
			return nil, errors.New("Codex returned an unsupported output item")
		}
		encoded, _ := json.Marshal(item.openAIResponsesOutput)
		normalized = append(normalized, encoded)
	}
	if !usable || (endTurn != nil && !*endTurn && !calls) {
		return nil, errors.New("Codex returned no complete agent decision")
	}
	return normalized, nil
}

func codexMetricsValid(response openAIResponsesResponse) bool {
	metrics := response.Metrics
	for _, value := range []*int{metrics.InputTokens, metrics.OutputTokens, metrics.TotalTokens, metrics.CachedInputTokens, metrics.ReasoningTokens} {
		if value != nil && *value < 0 {
			return false
		}
	}
	return metrics.CachedInputTokens == nil || metrics.InputTokens == nil || *metrics.CachedInputTokens <= *metrics.InputTokens
}
