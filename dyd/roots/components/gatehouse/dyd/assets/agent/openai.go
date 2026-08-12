package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type openAICompatibleRequest struct {
	Model           string                   `json:"model"`
	Messages        []openAICompatibleMessage `json:"messages"`
	Tools           []openAICompatibleTool    `json:"tools,omitempty"`
	ReasoningEffort string                   `json:"reasoning_effort,omitempty"`
}

type openAICompatibleMessage struct {
	Role       string                     `json:"role"`
	Content    string                     `json:"content"`
	ToolCallID string                     `json:"tool_call_id,omitempty"`
	ToolCalls  []openAICompatibleToolCall `json:"tool_calls,omitempty"`
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
	ParallelToolCalls bool                      `json:"parallel_tool_calls"`
	Reasoning         *openAIResponsesReasoning `json:"reasoning,omitempty"`
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
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

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
		return fmt.Errorf("OpenAI-compatible completion returned %s: %s", response.Status, strings.TrimSpace(string(contents))), openAICompatibleMessage{}
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
		return fmt.Errorf("OpenAI Responses returned %s: %s", response.Status, strings.TrimSpace(string(contents))), openAIResponsesResponse{}
	}
	var decoded openAIResponsesResponse
	if err := json.Unmarshal(contents, &decoded); err != nil {
		return fmt.Errorf("decode OpenAI Responses response: %w", err), openAIResponsesResponse{}
	}
	return nil, decoded
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
