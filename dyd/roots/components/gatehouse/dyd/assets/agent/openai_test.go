package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAICompatibleReply(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/chat/completions" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		var body openAICompatibleRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "test-model" || len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content != "hello" || len(body.Messages[0].ToolCalls) != 0 {
			t.Fatalf("request body = %#v", body)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Hello."}}]}`))
	}))
	defer server.Close()

	err, reply := OpenAICompatibleReply(context.Background(), server.Client(), server.URL+"/v1", "test-key", "test-model", []openAICompatibleMessage{{Role: "user", Content: "hello"}})
	if err != nil || reply != "Hello." {
		t.Fatalf("OpenAICompatibleReply() = (%v, %q)", err, reply)
	}
}

func TestOpenAICompatibleReasoningEffort(t *testing.T) {
	for _, test := range []struct {
		parameters string
		want       string
		valid      bool
	}{
		{parameters: `{}`, want: "", valid: true},
		{parameters: `{"reasoning_effort":"low"}`, want: "low", valid: true},
		{parameters: `{"reasoning_effort":"fast"}`, valid: false},
	} {
		err, effort := openAICompatibleReasoningEffort(test.parameters)
		if test.valid && (err != nil || effort != test.want) {
			t.Fatalf("openAICompatibleReasoningEffort(%s) = (%q, %v), want (%q, nil)", test.parameters, effort, err, test.want)
		}
		if !test.valid && err == nil {
			t.Fatalf("openAICompatibleReasoningEffort(%s) accepted invalid effort", test.parameters)
		}
	}
}

func TestOpenAIResponsesComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/responses" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		var body openAIResponsesRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "test-model" || body.Instructions != "test instructions" || body.Reasoning == nil || body.Reasoning.Effort != "low" || len(body.Input) != 1 || len(body.Tools) != 1 || body.Tools[0].Name != "lisp" || !body.Tools[0].Strict || body.ParallelToolCalls {
			t.Fatalf("request body = %#v", body)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"output":[{"type":"function_call","call_id":"call-1","name":"lisp","arguments":"{\"code\":\"(+ 1 2)\",\"reason\":\"test\"}"}]}`))
	}))
	defer server.Close()

	err, response := OpenAIResponsesComplete(context.Background(), server.Client(), server.URL+"/v1", "test-key", openAIResponsesRequest{
		Model: "test-model", Instructions: "test instructions", Input: []json.RawMessage{openAIResponsesMessage("user", "hello")}, Tools: []openAIResponsesTool{{Type: "function", Name: "lisp", Strict: true}}, Reasoning: &openAIResponsesReasoning{Effort: "low"},
	})
	if err != nil || len(response.Output) != 1 {
		t.Fatalf("OpenAIResponsesComplete() = (%#v, %v)", response, err)
	}
}
