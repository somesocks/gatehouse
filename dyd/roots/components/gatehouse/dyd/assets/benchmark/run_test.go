package benchmark

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gatehouse/internal/testdbos"
	"gatehouse/model"
)

func executeTest(ctx context.Context, configuration Config, path, version, fingerprint string, progress io.Writer) (error, *sql.DB) {
	return ExecuteWithOptions(ctx, configuration, path, version, fingerprint, progress, ExecutionOptions{
		Parallel: DefaultParallel, WorkflowPollingInterval: testdbos.PollInterval,
	})
}

func TestExecuteRejectsNegativeWorkflowPollingInterval(t *testing.T) {
	err, capture := ExecuteWithOptions(context.Background(), Config{}, "", "test", "build", io.Discard, ExecutionOptions{
		Parallel: DefaultParallel, WorkflowPollingInterval: -time.Millisecond,
	})
	if err == nil || capture != nil || !strings.Contains(err.Error(), "polling interval") {
		t.Fatalf("negative polling interval = %v, %#v", err, capture)
	}
}

func runConfiguration(t *testing.T, provider, selected, prompt, questions, environment string) (Config, string) {
	t.Helper()
	text := fmt.Sprintf(`api_version: v1
agent_providers:
  - %s
agent_models:
  - %s
system_prompts:
  - %s
questions:
%s
lisp_environments:
  - %s
tests:
  - alias: experiment
    matrix:
      models: [assistant]
      system_prompts: [prompt]
      questions: [question]
      lisp_environments: [environment]
`, provider, selected, prompt, questions, environment)
	path := filepath.Join(t.TempDir(), "tests.yaml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	err, configuration := ValidateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return configuration, path
}

func TestExecuteBuiltinAndSQLGrading(t *testing.T) {
	configuration, path := runConfiguration(t,
		`{alias: provider, protocol: builtin}`,
		`{alias: assistant, provider: provider, model: dummy.fixed-reply, parameters: {text: "24"}}`,
		`{alias: prompt, mode: builtin}`,
		`  - alias: question
    text: Return the temperature.
    expected: {output: "24", max_turns: 0, max_tool_calls: 0, max_tool_failures: 0}`,
		`{alias: environment, prelude: ""}`)
	repetitions := 2
	configuration.Document.Tests[0].Repetitions = &repetitions
	configuration.RunCount = 2
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err, capture := executeTest(ctx, configuration, path, "test", "fingerprint", io.Discard)
	if capture != nil {
		defer capture.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	var grade, output, status string
	var turns, calls int
	var tokens any
	if err := capture.QueryRow(`SELECT grade, output, status, turns, tool_calls, input_tokens FROM run_results`).Scan(&grade, &output, &status, &turns, &calls, &tokens); err != nil {
		t.Fatal(err)
	}
	if grade != "passed" || output != "24" || status != "completed" || turns != 0 || calls != 0 || tokens != nil {
		t.Fatalf("result = %s %q %s turns=%d calls=%d tokens=%v", grade, output, status, turns, calls, tokens)
	}
	if failed, err := ResultsFailed(ctx, capture); err != nil || failed {
		t.Fatalf("failed = %t, err = %v", failed, err)
	}
	var count int
	if err := capture.QueryRow(`SELECT COUNT(*) FROM session_events WHERE kind = 'message.text'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("question events = %d, %v", count, err)
	}
	if err := capture.QueryRow(`SELECT COUNT(DISTINCT session_id) FROM runs`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("run isolation = %d sessions, %v", count, err)
	}
	if err := capture.QueryRow(`SELECT COUNT(DISTINCT workspace_agent_id) FROM runs`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("binding reuse = %d agents, %v", count, err)
	}
	if err := capture.QueryRow(`SELECT COUNT(*) FROM execution_inputs WHERE json_extract(configuration, '$.questions[0].expected.max_tool_calls') = 0`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("captured expectations = %d, %v", count, err)
	}
}

func TestExecuteContinuesAfterNativeFailure(t *testing.T) {
	t.Setenv("BENCH_RUN_TEST_KEY", "original-key")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer original-key" {
			t.Error("selected provider credentials were not frozen for the execution")
		}
		if calls.Add(1) == 1 {
			_ = os.Setenv("BENCH_RUN_TEST_KEY", "changed-key")
			http.Error(w, "provider failure", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"output":[{"type":"message","status":"completed","content":[{"type":"output_text","text":"{\"text\":\"24\",\"attachments\":[]}"}]}]}`)
	}))
	defer server.Close()
	configuration, path := runConfiguration(t,
		fmt.Sprintf(`{alias: provider, protocol: openai-responses, endpoint: %q, api_key: {sources: ["env:BENCH_RUN_TEST_KEY"]}}`, server.URL),
		`{alias: assistant, provider: provider, model: test-model, parameters: {}}`,
		`{alias: prompt, mode: builtin}`, `  - {alias: question, text: hello, expected: {output: "24"}}`, `{alias: environment}`)
	repetitions := 2
	configuration.Document.Tests[0].Repetitions = &repetitions
	configuration.RunCount = 2
	err, capture := executeTest(context.Background(), configuration, path, "test", "build", io.Discard)
	if capture != nil {
		defer capture.Close()
	}
	if err == nil || capture == nil {
		t.Fatalf("failure execution = %v, capture = %v", err, capture)
	}
	var failed, passed int
	if err := capture.QueryRow(`SELECT SUM(grade = 'error'), SUM(grade = 'passed') FROM run_results`).Scan(&failed, &passed); err != nil {
		t.Fatal(err)
	}
	if failed != 1 || passed != 1 || calls.Load() != 2 {
		t.Fatalf("errors=%d passed=%d provider calls=%d", failed, passed, calls.Load())
	}
}

func TestExecuteCredentialReadCanBeCancelled(t *testing.T) {
	configuration, path := runConfiguration(t,
		`{alias: provider, protocol: openai-responses, endpoint: "https://unused.invalid", api_key: {sources: ["stdin:"]}}`,
		`{alias: assistant, provider: provider, model: test-model, parameters: {}}`,
		`{alias: prompt, mode: builtin}`, `  - {alias: question, text: hello}`, `{alias: environment}`)
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	previous := os.Stdin
	os.Stdin = reader
	defer func() { os.Stdin = previous }()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err, capture := executeTest(ctx, configuration, path, "test", "build", io.Discard)
	if capture != nil {
		defer capture.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) || capture == nil {
		t.Fatalf("credential cancellation = %v, capture = %v", err, capture)
	}
	var status string
	if err := capture.QueryRow(`SELECT status FROM executions`).Scan(&status); err != nil || status != "cancelled" {
		t.Fatalf("status = %s, err = %v", status, err)
	}
}

func TestExecuteScriptedProviders(t *testing.T) {
	for _, test := range []struct{ protocol, mode string }{
		{"openai-responses", "replace"}, {"openai-chat-completions", "append"}, {"openai-responses", "builtin"},
	} {
		t.Run(test.protocol+"/"+test.mode, func(t *testing.T) {
			t.Setenv("BENCH_RUN_TEST_KEY", "private-test-key")
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.RequestURI != "/custom/inference?deployment=test" || r.Header.Get("Authorization") != "Bearer private-test-key" {
					t.Errorf("incorrect transport: %s", r.RequestURI)
				}
				var request map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				var instructions string
				if test.protocol == "openai-responses" {
					instructions, _ = request["instructions"].(string)
				} else {
					messages := request["messages"].([]interface{})
					instructions, _ = messages[0].(map[string]interface{})["content"].(string)
				}
				if test.mode == "replace" && instructions != "Replacement instructions." {
					t.Errorf("replacement prompt = %q", instructions)
				}
				if test.mode == "append" && (!strings.HasSuffix(instructions, "Replacement instructions.") || !strings.Contains(instructions, "# Tools")) {
					t.Errorf("append prompt = %q", instructions)
				}
				if test.mode == "builtin" && (!strings.Contains(instructions, "# Tools") || strings.Contains(instructions, "Replacement instructions.")) {
					t.Errorf("builtin prompt = %q", instructions)
				}
				w.Header().Set("Content-Type", "application/json")
				var response interface{}
				if calls.Add(1) == 1 {
					programs := []string{`(error/throw "recoverable")`, `(import (fixture @native:fixture/v1) (begin (assert (int/gt (list/length (import/search "gatehouse/project")) 0)) fixture/value))`, `"` + strings.Repeat("x", 5000) + `"`}
					tools := []interface{}{}
					for index, program := range programs {
						arguments, _ := json.Marshal(map[string]string{"code": program, "reason": "exercise deterministic fixture"})
						if test.protocol == "openai-responses" {
							tools = append(tools, map[string]interface{}{"type": "function_call", "call_id": fmt.Sprint(index), "name": "lisp", "arguments": string(arguments)})
						} else {
							tools = append(tools, map[string]interface{}{"id": fmt.Sprint(index), "type": "function", "function": map[string]interface{}{"name": "lisp", "arguments": string(arguments)}})
						}
					}
					if test.protocol == "openai-responses" {
						response = map[string]interface{}{"output": tools}
					} else {
						response = map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "tool_calls": tools}}}}
					}
				} else {
					body := `{"text":"24","attachments":[]}`
					if test.protocol == "openai-responses" {
						response = map[string]interface{}{"output": []interface{}{map[string]interface{}{"type": "message", "status": "completed", "content": []interface{}{map[string]interface{}{"type": "output_text", "text": body}}}}, "usage": map[string]interface{}{"input_tokens": 10, "output_tokens": 2, "total_tokens": 12}}
					} else {
						response = map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "content": body}}}, "usage": map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12}}
					}
				}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			prompt := fmt.Sprintf(`{alias: prompt, mode: %s, text: "Replacement instructions."}`, test.mode)
			if test.mode == "builtin" {
				prompt = `{alias: prompt, mode: builtin}`
			}
			configuration, path := runConfiguration(t,
				fmt.Sprintf(`{alias: provider, protocol: %s, endpoint: %q, api_key: {sources: ["env:BENCH_RUN_TEST_KEY"], keychain: benchmark}}`, test.protocol, server.URL+"/custom/inference?deployment=test"),
				`{alias: assistant, provider: provider, model: test-model, parameters: {}, max_turns: 2, max_output_tokens: 1024}`,
				prompt,
				`  - alias: question
    text: Return the temperature.
    expected: {output: "24", max_turns: 1, max_tool_calls: 3, max_tool_failures: 1}`,
				`{alias: environment, project_linked: true, prelude: "(import/define (@native:fixture/v1 (list (pair 'value 24))) (eval program))"}`)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			err, capture := executeTest(ctx, configuration, path, "test", "fingerprint", io.Discard)
			if capture != nil {
				defer capture.Close()
			}
			if err != nil {
				failures, _ := resultRows(context.Background(), capture, `SELECT kind, payload FROM session_events WHERE kind LIKE '%.failure'`)
				t.Fatalf("%v; native failures: %v", err, failures)
			}
			var grade string
			var turns, tools, failures, tokens int
			if err := capture.QueryRow(`SELECT grade, turns, tool_calls, tool_failures, total_tokens FROM run_results`).Scan(&grade, &turns, &tools, &failures, &tokens); err != nil {
				t.Fatal(err)
			}
			if grade != "passed" || turns != 1 || tools != 3 || failures != 1 || tokens != 12 || calls.Load() != 2 {
				nativeFailures, _ := resultRows(ctx, capture, `SELECT payload FROM session_events WHERE kind = 'tool.failure'`)
				t.Fatalf("result=%s turns=%d calls=%d failures=%d tokens=%d provider calls=%d; %v", grade, turns, tools, failures, tokens, calls.Load(), nativeFailures)
			}
			var size int
			if err := capture.QueryRow(`SELECT length(content) FROM artifacts LIMIT 1`).Scan(&size); err != nil || size < 5000 {
				t.Fatalf("artifact size=%d, err=%v", size, err)
			}
			var output bytes.Buffer
			if err := Report(ctx, capture, "full", "json", &output); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), "private-test-key") {
				t.Fatal("report exposed provider credential")
			}
		})
	}
}

func TestExecuteCancellationCapturesPartialRun(t *testing.T) {
	t.Setenv("BENCH_RUN_TEST_KEY", "test")
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		started <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	configuration, path := runConfiguration(t,
		fmt.Sprintf(`{alias: provider, protocol: openai-responses, endpoint: %q, api_key: {sources: ["env:BENCH_RUN_TEST_KEY"]}}`, server.URL),
		`{alias: assistant, provider: provider, model: test-model, parameters: {}}`,
		`{alias: prompt, mode: builtin}`, `  - {alias: question, text: hello}`, `{alias: environment}`)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	go func() {
		select {
		case <-started:
			cancel()
		case <-ctx.Done():
		}
	}()
	err, capture := executeTest(ctx, configuration, path, "test", "build", io.Discard)
	if capture != nil {
		defer capture.Close()
	}
	if !errors.Is(err, context.Canceled) {
		failures, _ := resultRows(context.Background(), capture, `SELECT kind, payload FROM session_events WHERE kind LIKE '%.failure'`)
		t.Fatalf("Execute cancellation = %v; native failures: %v", err, failures)
	}
	if capture == nil {
		t.Fatal("lost partial capture")
	}
	var executionStatus, runStatus string
	if err := capture.QueryRow(`SELECT status FROM executions`).Scan(&executionStatus); err != nil {
		t.Fatal(err)
	}
	if err := capture.QueryRow(`SELECT status FROM runs`).Scan(&runStatus); err != nil {
		t.Fatal(err)
	}
	if executionStatus != "cancelled" || runStatus != "cancelled" {
		t.Fatalf("statuses = %s, %s", executionStatus, runStatus)
	}
	var count int
	if err := capture.QueryRow(`SELECT count(*) FROM session_events WHERE kind = 'cancel.success'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("native cancellation events = %d, %v", count, err)
	}
	if err := capture.QueryRow(`SELECT COUNT(*) FROM runs r JOIN session_events e ON e.id = r.terminal_event_id AND e.run_id = r.id WHERE e.kind = 'agent.failure'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("linked native terminal outcomes = %d, %v", count, err)
	}
}

func TestNativeCancellationTerminalOutcomes(t *testing.T) {
	request := &model.SessionEvent{Ref: model.SessionEventRef{Id: "cancel-request"}}
	other := model.SessionEventRef{Id: "other-request"}
	for _, test := range []struct {
		name, kind string
		parent     *model.SessionEventRef
		want       bool
	}{
		{"cancellation won", model.SessionEventKindCancelSuccess, &request.Ref, true},
		{"completion won", model.SessionEventKindCancelFailure, &request.Ref, true},
		{"unrelated success", model.SessionEventKindCancelSuccess, &other, false},
		{"unrelated failure", model.SessionEventKindCancelFailure, &other, false},
		{"missing parent", model.SessionEventKindCancelSuccess, nil, false},
		{"agent terminal alone", model.SessionEventKindAgentSuccess, &request.Ref, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			events := []model.SessionEvent{{Kind: test.kind, Parent: test.parent}}
			if got := nativeCancellationComplete(events, request); got != test.want {
				t.Fatalf("terminal cancellation outcome = %t, want %t", got, test.want)
			}
		})
	}
	if !nativeCancellationComplete(nil, nil) {
		t.Fatal("a run without a cancellation must not wait for one")
	}
}
