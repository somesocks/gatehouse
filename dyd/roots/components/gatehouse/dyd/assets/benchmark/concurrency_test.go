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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gatehouse/internal/testdbos"
)

func finalResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"output":[{"type":"message","status":"completed","content":[{"type":"output_text","text":"{\"text\":\"24\",\"attachments\":[]}"}]}]}`)
}

func TestExecuteConcurrencyAndRuntimeReuse(t *testing.T) {
	for _, limit := range []int{1, DefaultParallel} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			t.Setenv("BENCH_RUN_TEST_KEY", "test")
			started := make(chan struct{}, limit*2)
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			var active, peak, calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				count := active.Add(1)
				defer active.Add(-1)
				for previous := peak.Load(); count > previous; previous = peak.Load() {
					if peak.CompareAndSwap(previous, count) {
						break
					}
				}
				calls.Add(1)
				started <- struct{}{}
				select {
				case <-release:
					finalResponse(w)
				case <-r.Context().Done():
				}
			}))
			defer server.Close()
			defer unblock()
			configuration, path := runConfiguration(t,
				fmt.Sprintf(`{alias: provider, protocol: openai-responses, endpoint: %q, api_key: {sources: ["env:BENCH_RUN_TEST_KEY"]}}`, server.URL),
				`{alias: assistant, provider: provider, model: test-model, parameters: {}}`,
				`{alias: prompt, mode: builtin}`, `  - {alias: question, text: hello, expected: {output: "24"}}`, `{alias: environment}`)
			repetitions := limit * 2
			configuration.Document.Tests[0].Repetitions = &repetitions
			configuration.RunCount = uint64(repetitions)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			type result struct {
				err     error
				capture *sql.DB
			}
			done := make(chan result, 1)
			var progress bytes.Buffer
			go func() {
				var err error
				var capture *sql.DB
				if limit == DefaultParallel {
					err, capture = executeTest(ctx, configuration, path, "test", "build", &progress)
				} else {
					err, capture = ExecuteWithOptions(ctx, configuration, path, "test", "build", &progress, ExecutionOptions{Parallel: limit, WorkflowPollingInterval: testdbos.PollInterval})
				}
				done <- result{err, capture}
			}()
			for range limit {
				select {
				case <-started:
				case <-ctx.Done():
					unblock()
					outcome := <-done
					if outcome.capture != nil {
						outcome.capture.Close()
					}
					t.Fatalf("only %d requests overlapped: %v", peak.Load(), outcome.err)
				}
			}
			// While all slots are occupied, another matrix entry must stay queued.
			select {
			case <-started:
				t.Error("additional run started while all slots were occupied")
			case <-time.After(100 * time.Millisecond):
			}
			unblock()
			outcome := <-done
			if outcome.capture != nil {
				defer outcome.capture.Close()
			}
			if outcome.err != nil {
				t.Fatal(outcome.err)
			}
			if peak.Load() != int32(limit) || calls.Load() != int32(repetitions) {
				t.Fatalf("peak=%d calls=%d limit=%d repetitions=%d", peak.Load(), calls.Load(), limit, repetitions)
			}
			var passed, sessions, bindings int
			if err := outcome.capture.QueryRow(`SELECT SUM(grade = 'passed'), COUNT(DISTINCT session_id), COUNT(DISTINCT workspace_agent_id) FROM run_results`).Scan(&passed, &sessions, &bindings); err != nil {
				t.Fatal(err)
			}
			if passed != repetitions || sessions != repetitions || bindings != 1 {
				t.Fatalf("passed=%d sessions=%d bindings=%d", passed, sessions, bindings)
			}
			if count := strings.Count(progress.String(), `msg="DBOS launched"`); count != 1 {
				t.Fatalf("DBOS launched %d times", count)
			}
		})
	}
}

func TestExecuteConcurrentPromptsEnvironmentsAndArtifacts(t *testing.T) {
	t.Setenv("BENCH_RUN_TEST_KEY", "test")
	var mutex sync.Mutex
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model        string          `json:"model"`
			Instructions string          `json:"instructions"`
			Input        json.RawMessage `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		prompt := request.Instructions
		if prompt != "replacement-a" && prompt != "replacement-b" {
			if !strings.Contains(prompt, "# Tools") || !strings.Contains(prompt, "@assistant") {
				t.Error("builtin/append prompt lost its stable assistant identity")
			}
			if strings.HasSuffix(prompt, "append-marker") {
				prompt = "append"
			} else {
				prompt = "builtin"
			}
		}
		mutex.Lock()
		counts[request.Model+"/"+prompt]++
		mutex.Unlock()
		if bytes.Contains(request.Input, []byte(`"function_call_output"`)) {
			finalResponse(w)
			return
		}
		arguments, _ := json.Marshal(map[string]string{
			"code":   `(string/concat fixture "` + strings.Repeat("x", 5000) + `")`,
			"reason": "check per-environment fixture and artifact",
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"output": []any{map[string]any{
			"type": "function_call", "call_id": "fixture", "name": "lisp", "arguments": string(arguments),
		}}})
	}))
	defer server.Close()
	configuration, path := runConfiguration(t,
		fmt.Sprintf(`{alias: provider, protocol: openai-responses, endpoint: %q, api_key: {sources: ["env:BENCH_RUN_TEST_KEY"]}}`, server.URL),
		`{alias: assistant, provider: provider, model: first-model, parameters: {}, max_turns: 2}`,
		`{alias: prompt, mode: builtin}`, `  - {alias: question, text: hello, expected: {output: "24", max_tool_calls: 1, max_tool_failures: 0}}`,
		`{alias: environment, prelude: '(let ((fixture "left-marker")) (eval program))'}`)
	configuration.Document.LispEnvironments[0].Alias = "left"
	otherModel := configuration.Document.AgentModels[0]
	otherModel.Alias, otherModel.Model = "other", "second-model"
	configuration.Document.AgentModels = append(configuration.Document.AgentModels, otherModel)
	for _, specification := range [][3]string{{"appended", "append", "append-marker"}, {"replace-a", "replace", "replacement-a"}, {"replace-b", "replace", "replacement-b"}} {
		prompt := configuration.Document.SystemPrompts[0]
		prompt.Alias, prompt.Mode, prompt.Text = specification[0], specification[1], stringPointer(specification[2])
		configuration.Document.SystemPrompts = append(configuration.Document.SystemPrompts, prompt)
	}
	otherEnvironment := configuration.Document.LispEnvironments[0]
	otherEnvironment.Alias, otherEnvironment.Prelude = "right", stringPointer(`(let ((fixture "right-marker")) (eval program))`)
	configuration.Document.LispEnvironments = append(configuration.Document.LispEnvironments, otherEnvironment)
	matrix := &configuration.Document.Tests[0].Matrix
	matrix.Models = []string{"assistant", "other"}
	matrix.SystemPrompts = []string{"prompt", "appended", "replace-a", "replace-b"}
	matrix.LispEnvironments = []string{"left", "right"}
	configuration.RunCount = 16
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	err, capture := executeTest(ctx, configuration, path, "test", "build", io.Discard)
	if capture != nil {
		defer capture.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	var passed, artifacts, mismatches int
	if err := capture.QueryRow(`SELECT SUM(grade = 'passed') FROM run_results`).Scan(&passed); err != nil {
		t.Fatal(err)
	}
	if err := capture.QueryRow(`SELECT COUNT(*) FROM artifacts a JOIN run_results r ON r.id = a.run_id WHERE instr(CAST(a.content AS TEXT), r.environment_alias || '-marker') > 0`).Scan(&artifacts); err != nil {
		t.Fatal(err)
	}
	if err := capture.QueryRow(`SELECT COUNT(*) FROM session_events e JOIN runs r ON r.id = e.run_id WHERE e.session <> r.session_id OR e.workspace <> r.workspace_id`).Scan(&mismatches); err != nil {
		t.Fatal(err)
	}
	if passed != 16 || artifacts != 16 || mismatches != 0 {
		t.Fatalf("passed=%d matching artifacts=%d mismatched events=%d", passed, artifacts, mismatches)
	}
	for _, model := range []string{"first-model", "second-model"} {
		for _, prompt := range []string{"builtin", "append", "replacement-a", "replacement-b"} {
			if count := counts[model+"/"+prompt]; count != 4 {
				t.Errorf("%s/%s: %d requests, want 4", model, prompt, count)
			}
		}
	}
}

func TestExecuteCancellationDrainsConcurrentRuns(t *testing.T) {
	t.Setenv("BENCH_RUN_TEST_KEY", "test")
	started := make(chan struct{}, 16)
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		calls.Add(1)
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
	repetitions := 16
	configuration.Document.Tests[0].Repetitions, configuration.RunCount = &repetitions, 16
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	go func() {
		for range DefaultParallel {
			select {
			case <-started:
			case <-ctx.Done():
				return
			}
		}
		cancel()
	}()
	err, capture := executeTest(ctx, configuration, path, "test", "build", io.Discard)
	if capture != nil {
		defer capture.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	var runs, cancelled, terminals int
	if err := capture.QueryRow(`SELECT COUNT(*), SUM(status = 'cancelled') FROM runs`).Scan(&runs, &cancelled); err != nil {
		t.Fatal(err)
	}
	if err := capture.QueryRow(`SELECT COUNT(*) FROM runs r JOIN session_events e ON e.id = r.terminal_event_id AND e.run_id = r.id WHERE e.kind = 'agent.failure'`).Scan(&terminals); err != nil {
		t.Fatal(err)
	}
	if runs != DefaultParallel || cancelled != runs || terminals != runs || calls.Load() != int32(runs) {
		t.Fatalf("runs=%d cancelled=%d terminals=%d requests=%d", runs, cancelled, terminals, calls.Load())
	}
	var status string
	if err := capture.QueryRow(`SELECT status FROM executions`).Scan(&status); err != nil || status != "cancelled" {
		t.Fatalf("status=%s: %v", status, err)
	}
}
