package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type openAICompatibleRequest struct {
	Model             string                   `json:"model"`
	Messages          []openAICompatibleMessage `json:"messages"`
	Tools             []openAICompatibleTool    `json:"tools,omitempty"`
	ResponseFormat    *openAIResponseFormat     `json:"response_format,omitempty"`
	ParallelToolCalls bool                     `json:"parallel_tool_calls"`
	ReasoningEffort   string                   `json:"reasoning_effort,omitempty"`
	MaxTokens         int                      `json:"max_tokens,omitempty"`
}

type openAIResponseFormat struct {
	Type       string                 `json:"type"`
	JSONSchema openAIJSONSchemaFormat `json:"json_schema"`
}

type openAIJSONSchemaFormat struct {
	Type   string `json:"type,omitempty"`
	Name   string `json:"name"`
	Strict bool   `json:"strict"`
	Schema any    `json:"schema"`
}

type openAICompatibleMessage struct {
	Role       string                     `json:"role"`
	Content    string                     `json:"content"`
	ToolCallID string                     `json:"tool_call_id,omitempty"`
	ToolCalls  []openAICompatibleToolCall `json:"tool_calls,omitempty"`
	Message    *transcriptMessage         `json:"-"`
	ToolOutput *transcriptToolOutput      `json:"-"`
}

type openAICompatibleTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

type openAICompatibleToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAICompatibleResponse struct {
	Choices []struct {
		Message openAICompatibleMessage `json:"message"`
	} `json:"choices"`
}

type openAIResponsesRequest struct {
	Model             string                    `json:"model"`
	Instructions      string                    `json:"instructions,omitempty"`
	Input             []json.RawMessage         `json:"input"`
	Tools             []openAIResponsesTool     `json:"tools,omitempty"`
	Text              *openAIResponsesText      `json:"text,omitempty"`
	ParallelToolCalls bool                      `json:"parallel_tool_calls"`
	Reasoning         *openAIResponsesReasoning `json:"reasoning,omitempty"`
	MaxOutputTokens   int                       `json:"max_output_tokens,omitempty"`
}

type openAIResponsesText struct {
	Format openAIJSONSchemaFormat `json:"format"`
}

type openAIResponsesReasoning struct {
	Effort string `json:"effort"`
}

type openAIResponsesTool struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
	Strict      bool   `json:"strict"`
}

type openAIResponsesResponse struct {
	Output []json.RawMessage `json:"output"`
}

type openAIResponsesOutput struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Status    string `json:"status"`
	Content   []openAIResponsesOutputContent `json:"content"`
}

type openAIResponsesOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openAIRetryableError struct {
	message    string
	retryAfter time.Duration
}

func (err *openAIRetryableError) Error() string { return err.message }

func (err *openAIRetryableError) RetryAfter() time.Duration { return err.retryAfter }

func OpenAICompatibleReply(ctx context.Context, client *http.Client, baseURL, apiKey, model string, messages []openAICompatibleMessage) (error, string) {
	err, message := OpenAICompatibleComplete(ctx, client, baseURL, apiKey, openAICompatibleRequest{Model: model, Messages: messages})
	if err != nil {
		return err, ""
	}
	if len(message.ToolCalls) != 0 || strings.TrimSpace(message.Content) == "" {
		return fmt.Errorf("OpenAI-compatible completion returned no message"), ""
	}
	return nil, message.Content
}

func OpenAICompatibleComplete(ctx context.Context, client *http.Client, baseURL, apiKey string, completion openAICompatibleRequest) (error, openAICompatibleMessage) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("parse OpenAI-compatible base URL: %w", err), openAICompatibleMessage{}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/chat/completions"
	payload, err := json.Marshal(completion)
	if err != nil {
		return fmt.Errorf("encode OpenAI-compatible request: %w", err), openAICompatibleMessage{}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create OpenAI-compatible request: %w", err), openAICompatibleMessage{}
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request OpenAI-compatible completion: %w", err), openAICompatibleMessage{}
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read OpenAI-compatible response: %w", err), openAICompatibleMessage{}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return openAICompletionError("OpenAI-compatible completion", response, contents), openAICompatibleMessage{}
	}
	var decoded openAICompatibleResponse
	if err := json.Unmarshal(contents, &decoded); err != nil {
		return fmt.Errorf("decode OpenAI-compatible response: %w", err), openAICompatibleMessage{}
	}
	if len(decoded.Choices) == 0 {
		return fmt.Errorf("OpenAI-compatible completion returned no choices"), openAICompatibleMessage{}
	}
	return nil, decoded.Choices[0].Message
}

func OpenAIResponsesComplete(ctx context.Context, client *http.Client, baseURL, apiKey string, completion openAIResponsesRequest) (error, openAIResponsesResponse) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("parse OpenAI Responses base URL: %w", err), openAIResponsesResponse{}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/responses"
	payload, err := json.Marshal(completion)
	if err != nil {
		return fmt.Errorf("encode OpenAI Responses request: %w", err), openAIResponsesResponse{}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create OpenAI Responses request: %w", err), openAIResponsesResponse{}
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request OpenAI Responses: %w", err), openAIResponsesResponse{}
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read OpenAI Responses response: %w", err), openAIResponsesResponse{}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return openAICompletionError("OpenAI Responses", response, contents), openAIResponsesResponse{}
	}
	var decoded openAIResponsesResponse
	if err := json.Unmarshal(contents, &decoded); err != nil {
		return fmt.Errorf("decode OpenAI Responses response: %w", err), openAIResponsesResponse{}
	}
	return nil, decoded
}

func openAICompletionError(name string, response *http.Response, contents []byte) error {
	message := fmt.Sprintf("%s returned %s: %s", name, response.Status, strings.TrimSpace(string(contents)))
	if response.StatusCode != http.StatusTooManyRequests {
		return fmt.Errorf("%s", message)
	}
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(contents, &payload) != nil || payload.Error.Code != "rate_limit_exceeded" {
		return fmt.Errorf("%s", message)
	}
	retryAfter, ok := openAIRetryAfter(response.Header)
	if !ok {
		return fmt.Errorf("%s", message)
	}
	return &openAIRetryableError{message: message, retryAfter: retryAfter}
}

func openAIRetryAfter(headers http.Header) (time.Duration, bool) {
	if milliseconds, err := strconv.ParseInt(headers.Get("retry-after-ms"), 10, 64); err == nil && milliseconds > 0 {
		return time.Duration(milliseconds) * time.Millisecond, true
	}
	value := headers.Get("Retry-After")
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second, true
	}
	deadline, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	duration := time.Until(deadline)
	return duration, duration > 0
}

func openAIResponsesMessage(role, content string) json.RawMessage {
	encoded, _ := json.Marshal(struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: role, Content: content})
	return encoded
}

func openAIResponsesFunctionOutput(callID, output string) json.RawMessage {
	encoded, _ := json.Marshal(struct {
		Type   string `json:"type"`
		CallID string `json:"call_id"`
		Output string `json:"output"`
	}{Type: "function_call_output", CallID: callID, Output: output})
	return encoded
}

func openAIResponsesFunctionCall(call openAICompatibleToolCall) json.RawMessage {
	encoded, _ := json.Marshal(struct {
		Type      string `json:"type"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Type: "function_call", CallID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
	return encoded
}
