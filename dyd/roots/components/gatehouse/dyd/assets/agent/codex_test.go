package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"gatehouse/codexlogin"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/model"
)

type codexTestTransport func(*http.Request) (*http.Response, error)

func (fn codexTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func codexTestCredential() codexlogin.Credential {
	return codexlogin.Credential{Version: 1, AccessToken: "CODEX_ACCESS_SENTINEL", RefreshToken: "CODEX_REFRESH_SENTINEL", AccountID: "account"}
}

func codexTestEvent(kind, fields string) string {
	return "event: " + kind + "\r\ndata: {\"type\":\"" + kind + "\"," + fields + "}\r\n\r\n"
}

func codexTestMessage(text string) string {
	encoded, _ := json.Marshal(text)
	return `{"type":"message","role":"assistant","phase":"final_answer","content":[{"type":"output_text","text":` + string(encoded) + `}]}`
}

func codexTestStream() string {
	return ": keep-alive\r\n\r\n" + codexTestEvent("response.output_text.delta", `"delta":"not a committed decision"`) +
		codexTestEvent("response.output_item.done", `"output_index":0,"item":`+codexTestMessage(`{"text":"Done.","attachments":[]}`)) +
		codexTestEvent("response.completed", `"response":{"id":"resp_test","status":"completed","usage":{"input_tokens":100,"output_tokens":30,"total_tokens":130,"input_tokens_details":{"cached_tokens":50},"output_tokens_details":{"reasoning_tokens":20}}}`)
}

func TestCodexRequestAndStreamingCompletion(t *testing.T) {
	endpoint, encrypted := config.OpenAICodexEndpoint, "encrypted"
	selected := &database.WorkspaceAgentModel{Alias: "assistant", Protocol: config.AgentProviderProtocolOpenAICodex,
		Endpoint: &endpoint, APIKey: &encrypted, Keychain: &model.KeychainRef{Id: "test", Version: 1},
		Model: "gpt-codex", Parameters: `{"reasoning_effort":"high"}`, MaxOutputTokens: 100}
	err, request := agentProviderRequestPrepare(selected, false, []openAICompatibleMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &http.Client{Transport: codexTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != config.OpenAICodexEndpoint || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer CODEX_ACCESS_SENTINEL" || r.Header.Get("ChatGPT-Account-ID") != "account" || r.Header.Get("Accept") != "text/event-stream" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("incorrect Codex endpoint or headers")
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "CODEX_") {
			t.Error("credentials in request body")
		}
		var decoded map[string]any
		if json.Unmarshal(body, &decoded) != nil || decoded["model"] != "gpt-codex" || decoded["stream"] != true || decoded["store"] != false || decoded["tool_choice"] != "auto" || decoded["max_output_tokens"] != nil || decoded["instructions"] == "" || decoded["text"] == nil || decoded["tools"] == nil {
			t.Errorf("incorrect Codex wire body: %s", body)
		}
		estimated, _ := json.Marshal(request.body())
		if string(estimated) != string(body) {
			t.Error("rate estimate differs from wire body")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(codexTestStream()))}, nil
	})}
	result, err := codexComplete(context.Background(), client, codexTestCredential(), *request.Responses)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(result.Output) != 1 || *result.Metrics.InputTokens != 100 || *result.Metrics.CachedInputTokens != 50 || *result.Metrics.OutputTokens != 30 || *result.Metrics.ReasoningTokens != 20 || *result.Metrics.TotalTokens != 130 {
		t.Fatal("missing completion or usage")
	}
	var output openAIResponsesOutput
	if json.Unmarshal(result.Output[0], &output) != nil {
		t.Fatal("invalid output")
	}
	final, err := openAIResponsesFinalReply([]openAIResponsesOutput{output})
	if err != nil || final.Text != "Done." {
		t.Fatalf("final reply: %v", err)
	}
}

func TestCodexRejectsUnfinishedInvalidAndOversizedStreams(t *testing.T) {
	message := codexTestMessage(`{"text":"Partial","attachments":[]}`)
	item := codexTestEvent("response.output_item.done", `"output_index":0,"item":`+message)
	for name, stream := range map[string]string{
		"empty": "", "items without completion": item,
		"done without completion":         item + "data: [DONE]\n\n",
		"invalid JSON":                    "data: CODEX_ACCESS_SENTINEL\n\n",
		"failed after tool":               codexTestEvent("response.output_item.done", `"item":{"type":"function_call","call_id":"call_1","name":"lisp","arguments":"{}"}`) + codexTestEvent("response.failed", `"response":{"error":{"message":"CODEX_REFRESH_SENTINEL"}}`),
		"incomplete":                      item + codexTestEvent("response.incomplete", `"response":{"id":"resp","status":"incomplete"}`),
		"error":                           codexTestEvent("error", `"error":{"message":"CODEX_REFRESH_SENTINEL"}`),
		"missing terminal frame boundary": item + `data: {"type":"response.completed","response":{"id":"resp"}}`,
		"completion with failed status":   item + codexTestEvent("response.completed", `"response":{"id":"resp","status":"failed"}`),
		"negative item index":             strings.ReplaceAll(item, `"output_index":0`, `"output_index":-1`) + codexTestEvent("response.completed", `"response":{"id":"resp"}`),
		"duplicate item":                  item + item + codexTestEvent("response.completed", `"response":{"id":"resp"}`),
		"commentary only":                 codexTestEvent("response.completed", `"response":{"id":"resp","output":[`+strings.ReplaceAll(message, "final_answer", "commentary")+`]}`),
		"unfinished turn":                 item + codexTestEvent("response.completed", `"response":{"id":"resp","end_turn":false}`),
		"invalid usage":                   item + codexTestEvent("response.completed", `"response":{"id":"resp","usage":{"input_tokens":-1}}`),
		"line too large":                  "data: " + strings.Repeat("a", codexMaximumEventBytes) + "\n\n",
		"frame too large":                 strings.Repeat("data: "+strings.Repeat(" ", 1024)+"\n", 1024),
		"stream too large":                strings.Repeat(": heartbeat\n\n", codexMaximumStreamBytes/13+1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := codexReadStream(strings.NewReader(stream), time.Now())
			if err == nil || strings.Contains(err.Error(), "SENTINEL") {
				t.Fatalf("unsafe or missing failure: %v", err)
			}
		})
	}
}

func TestCodexToolItemsAndFinalPhases(t *testing.T) {
	tool := `{"type":"function_call","call_id":"call_1","name":"lisp","arguments":"{\"code\":\"42\",\"reason\":\"test\"}"}`
	stream := codexTestEvent("response.function_call_arguments.delta", `"delta":"incomplete"`) +
		codexTestEvent("response.output_item.done", `"output_index":1,"item":`+tool) +
		codexTestEvent("response.completed", `"response":{"id":"resp","end_turn":false,"output":[]}`)
	result, err := codexReadStream(strings.NewReader(stream), time.Now())
	if err != nil || len(result.Output) != 1 {
		t.Fatalf("tool completion: %v", err)
	}
	for _, outputs := range []string{"[" + tool + "," + tool + "]", `[{"type":"web_search_call"}]`} {
		_, err := codexReadStream(strings.NewReader(codexTestEvent("response.completed", `"response":{"id":"resp","output":`+outputs+`}`)), time.Now())
		if err == nil {
			t.Fatal("invalid tool output accepted")
		}
	}
	final := codexTestMessage(`{"text":"Final","attachments":[]}`)
	commentary := strings.ReplaceAll(codexTestMessage(`{"text":"Working","attachments":[]}`), "final_answer", "commentary")
	stream = codexTestEvent("response.completed", `"response":{"id":"resp","output":[{"type":"reasoning","encrypted_content":"PRIVATE_REASONING"},`+commentary+","+final+`]}`)
	result, err = codexReadStream(strings.NewReader(stream), time.Now())
	if err != nil || len(result.Output) != 1 || strings.Contains(string(result.Output[0]), "Working") {
		t.Fatalf("wrong final phase: %v", err)
	}
}

func TestCodexMultilineCompletionDoesNotWaitForEOF(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	stream := "id: ignored\n: heartbeat\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\ndata: \"response\":{\"id\":\"resp\",\"output\":[" + codexTestMessage(`{"text":"Done","attachments":[]}`) + "]}}\n\n"
	go func() { io.WriteString(writer, stream) }()
	done := make(chan error, 1)
	go func() { _, err := codexReadStream(reader, time.Now()); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("completion waited for the server to close the stream")
	}
}

func TestCodexHTTPFailuresNeverRefreshRetryOrLeak(t *testing.T) {
	for _, status := range []int{301, 307, 400, 401, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: codexTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Header: http.Header{"Location": {"https://other.example/"}}, Body: io.NopCloser(strings.NewReader("CODEX_ACCESS_SENTINEL CODEX_REFRESH_SENTINEL")), Request: r}, nil
			})}
			_, err := codexComplete(context.Background(), client, codexTestCredential(), openAIResponsesRequest{Model: "test"})
			if err == nil || calls != 1 || strings.Contains(err.Error(), "SENTINEL") {
				t.Fatalf("incorrect failure: calls=%d err=%v", calls, err)
			}
		})
	}
	client := &http.Client{Transport: codexTestTransport(func(r *http.Request) (*http.Response, error) { return nil, errors.New("CODEX_ACCESS_SENTINEL") })}
	if _, err := codexComplete(context.Background(), client, codexTestCredential(), openAIResponsesRequest{}); err == nil || strings.Contains(err.Error(), "SENTINEL") {
		t.Fatal("transport error was not sanitized")
	}
}

func TestCodexCancellationAndOutputLimit(t *testing.T) {
	client := &http.Client{Transport: codexTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(codexTestStream()))}, nil
	})}
	result, err := codexComplete(context.Background(), client, codexTestCredential(), openAIResponsesRequest{MaxOutputTokens: 10})
	if err == nil || result.Metrics == nil || *result.Metrics.OutputTokens != 30 {
		t.Fatal("output limit lost actual usage or accepted oversized output")
	}
	ctx, cancel := context.WithCancel(context.Background())
	client.Transport = codexTestTransport(func(r *http.Request) (*http.Response, error) {
		cancel()
		return nil, r.Context().Err()
	})
	if _, err := codexComplete(ctx, client, codexTestCredential(), openAIResponsesRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
