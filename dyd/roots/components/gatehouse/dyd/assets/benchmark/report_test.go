package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gatehouse/benchmarkmodel"
	"gatehouse/typed_id"
)

func TestSQLExpectationChecks(t *testing.T) {
	for _, test := range []struct {
		name, want string
		change     func(*resultsFixture)
	}{
		{"inclusive limits and Unicode trim", "passed", func(f *resultsFixture) {}},
		{"wrong output", "failed", func(f *resultsFixture) { f.question.ExpectedOutput = stringPointer("different") }},
		{"turn budget", "failed", func(f *resultsFixture) { value := 0; f.question.ExpectedMaxTurns = &value }},
		{"tool budget", "failed", func(f *resultsFixture) { value := 0; f.question.ExpectedMaxToolCalls = &value }},
		{"tool failure budget", "failed", func(f *resultsFixture) { f.events[3].Kind = "tool.failure" }},
		{"ungraded", "ungraded", func(f *resultsFixture) {
			f.question.ExpectedOutput, f.question.ExpectedMaxTurns, f.question.ExpectedMaxToolCalls, f.question.ExpectedMaxToolFailures = nil, nil, nil, nil
		}},
		{"execution failure", "error", func(f *resultsFixture) { f.run.Status = "failed" }},
		{"cancelled", "cancelled", func(f *resultsFixture) { f.run.Status = "cancelled" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture := openTestResults(t)
			f := newResultsFixture(t, "question")
			zero, one := 0, 1
			f.question.ExpectedOutput = stringPointer("\tCapabilities\u2003")
			f.question.ExpectedMaxTurns, f.question.ExpectedMaxToolCalls, f.question.ExpectedMaxToolFailures = &one, &one, &zero
			id, _ := typed_id.New(typed_id.SessionEvent)
			metrics := map[string]interface{}{"input_tokens": 0, "output_tokens": 0}
			f.events = append(f.events, benchmarkmodel.SessionEvent{Id: id, RunId: f.run.Id, Workspace: *f.run.WorkspaceId, Session: *f.run.SessionId,
				Parent: f.run.AgentRequestId, Kind: "thinking.update", Payload: map[string]interface{}{"reason": "rate_limit"}, Metrics: &metrics, CreatedAt: "2026-10-13T12:00:02Z"})
			id, _ = typed_id.New(typed_id.SessionEvent)
			f.events = append(f.events, benchmarkmodel.SessionEvent{Id: id, RunId: f.run.Id, Workspace: *f.run.WorkspaceId, Session: *f.run.SessionId,
				Parent: f.run.AgentRequestId, Kind: "thinking.success", Payload: map[string]interface{}{"tool_requests": []interface{}{map[string]interface{}{"id": f.events[2].Id}}}, CreatedAt: "2026-10-13T12:00:02Z"})
			test.change(&f)
			insertResultFixture(t, capture, f)
			var grade string
			var turns, elapsed, tokens, outcomes, covered int
			if err := capture.QueryRow(`SELECT grade, turns, elapsed_ms, input_tokens, provider_outcomes, outcomes_with_metrics FROM run_results`).Scan(&grade, &turns, &elapsed, &tokens, &outcomes, &covered); err != nil {
				t.Fatal(err)
			}
			if grade != test.want || turns != 1 || elapsed != 4000 || tokens != 0 || outcomes != 2 || covered != 1 {
				t.Fatalf("grade=%s turns=%d elapsed=%d tokens=%d coverage=%d/%d", grade, turns, elapsed, tokens, covered, outcomes)
			}
			var inputCoverage, reasoningCoverage int
			if err := capture.QueryRow(`SELECT input_tokens_observations, reasoning_tokens_observations FROM run_results`).Scan(&inputCoverage, &reasoningCoverage); err != nil {
				t.Fatal(err)
			}
			if inputCoverage != 1 || reasoningCoverage != 0 {
				t.Fatalf("field coverage = %d input, %d reasoning", inputCoverage, reasoningCoverage)
			}
		})
	}
}

func TestReportFormatsFromSameCapture(t *testing.T) {
	capture := openTestResults(t)
	f := newResultsFixture(t, "question")
	f.execution.Status = "completed"
	f.question.ExpectedOutput = stringPointer("Capabilities")
	insertResultFixture(t, capture, f)
	content := []byte{0, 1, 2, 255}
	if _, err := capture.Exec(`INSERT INTO artifacts (run_id, id, metadata, content) VALUES (?, 'file', '{"name":"fixture"}', ?)`, f.run.Id, content); err != nil {
		t.Fatal(err)
	}
	for _, report := range []string{"summary", "results", "full"} {
		for _, format := range []string{"text", "json", "jsonl", "sqlite"} {
			t.Run(report+"/"+format, func(t *testing.T) {
				var output bytes.Buffer
				err := Report(context.Background(), capture, report, format, &output)
				if format == "sqlite" && report != "full" {
					if err == nil || output.Len() != 0 {
						t.Fatal("invalid SQLite combination emitted output")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				switch format {
				case "json":
					var document map[string]interface{}
					if err := json.Unmarshal(output.Bytes(), &document); err != nil {
						t.Fatal(err)
					}
					if document["report"] != report {
						t.Fatal("report type lost")
					}
					if report == "full" {
						data := document["data"].(map[string]interface{})
						artifact := data["artifacts"].([]interface{})[0].(map[string]interface{})
						if artifact["content"] != "AAEC/w==" || !strings.Contains(document["schema"].(string), "CREATE VIEW run_results") {
							t.Fatal("full report lost artifact or grading schema")
						}
					}
				case "jsonl":
					decoder := json.NewDecoder(&output)
					found := map[string]bool{}
					for {
						var record map[string]interface{}
						if err := decoder.Decode(&record); err != nil {
							if errors.Is(err, io.EOF) {
								break
							}
							t.Fatal(err)
						}
						found[record["type"].(string)] = true
					}
					if !found["summary"] || report != "summary" && !found["result"] || report == "full" && (!found["schema"] || !found["artifacts"]) {
						t.Fatalf("missing JSONL records: %v", found)
					}
				case "sqlite":
					if !bytes.HasPrefix(output.Bytes(), []byte("SQLite format 3\x00")) {
						t.Fatal("stdout is not a SQLite database")
					}
					path := filepath.Join(t.TempDir(), "results.sqlite")
					if err := os.WriteFile(path, output.Bytes(), 0o600); err != nil {
						t.Fatal(err)
					}
					exported := openResultsFileForTest(t, path)
					assertResultsFileEqual(t, capture, exported)
					var grade string
					if err := exported.QueryRow(`SELECT grade FROM run_results`).Scan(&grade); err != nil || grade != "passed" {
						t.Fatalf("exported grade = %s, %v", grade, err)
					}
				default:
					if output.Len() == 0 {
						t.Fatal("empty text output")
					}
				}
			})
		}
	}
}

type failingReportWriter struct{}

func (failingReportWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestReportPropagatesOutputFailure(t *testing.T) {
	capture := openTestResults(t)
	insertResultFixture(t, capture, newResultsFixture(t, "question"))
	for _, report := range []string{"summary", "results", "full"} {
		for _, format := range []string{"text", "json", "jsonl", "sqlite"} {
			if format == "sqlite" && report != "full" {
				continue
			}
			if err := Report(context.Background(), capture, report, format, failingReportWriter{}); !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("%s/%s output error = %v", report, format, err)
			}
		}
	}
}

func TestExpectedValidationAndZeroPreservation(t *testing.T) {
	for _, expected := range []string{`{}`, `{max_turns: -1}`, `{max_tool_failures: -1}`, `{max_tool_calls: -1}`, `{output: 24}`} {
		directory := t.TempDir()
		_, path := runConfiguration(t, `{alias: provider, protocol: builtin}`, `{alias: assistant, provider: provider, model: dummy.fixed-reply, parameters: {text: reply}}`, `{alias: prompt, mode: builtin}`, `  - {alias: question, text: hello}`, `{alias: environment}`)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		bad := strings.Replace(string(data), "text: hello}", "text: hello, expected: "+expected+"}", 1)
		path = filepath.Join(directory, "tests.yaml")
		if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		if err, _ := ValidateFile(path); err == nil {
			t.Errorf("accepted invalid expected %s", expected)
		}
	}
	// Explicit zero is also exercised by the native builtin execution test.
}

func TestReportTextExplainsChecksAndPartialMeasurements(t *testing.T) {
	capture := openTestResults(t)
	fixtures := []resultsFixture{newResultsFixture(t, "first"), newResultsFixture(t, "second")}
	for index := range fixtures {
		f := &fixtures[index]
		f.execution.Status = "completed"
		f.run.TestAlias = []string{"passing-test", "tool-budget-test"}[index]
		zero, one, two := 0, 1, 2
		f.question.ExpectedOutput = stringPointer("Capabilities")
		f.question.ExpectedMaxTurns, f.question.ExpectedMaxToolFailures = &two, &zero
		f.question.ExpectedMaxToolCalls = &one
		if index == 1 {
			f.question.ExpectedMaxToolCalls = &zero
		}
		metrics := map[string]interface{}{"input_tokens": 10, "output_tokens": 2, "request_ms": 20, "reasoning_tokens": 0, "cached_input_tokens": 0, "total_tokens": 12}
		id, _ := typed_id.New(typed_id.SessionEvent)
		f.events = append(f.events, benchmarkmodel.SessionEvent{Id: id, RunId: f.run.Id, Workspace: *f.run.WorkspaceId, Session: *f.run.SessionId,
			Parent: f.run.AgentRequestId, Kind: "thinking.success", Payload: map[string]interface{}{"tool_requests": []interface{}{map[string]interface{}{"id": f.events[2].Id}}}, Metrics: &metrics, CreatedAt: "2026-10-13T12:00:02Z"})
		id, _ = typed_id.New(typed_id.SessionEvent)
		f.events = append(f.events, benchmarkmodel.SessionEvent{Id: id, RunId: f.run.Id, Workspace: *f.run.WorkspaceId, Session: *f.run.SessionId,
			Parent: f.run.AgentRequestId, Kind: "thinking.success", Payload: map[string]interface{}{"agent_success": map[string]interface{}{"id": *f.run.TerminalEventId}}, CreatedAt: "2026-10-13T12:00:04Z"})
		insertResultFixture(t, capture, *f)
	}
	var summary bytes.Buffer
	if err := Report(context.Background(), capture, "summary", "text", &summary); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		found := false
		for _, line := range strings.Split(summary.String(), "\n") {
			if strings.Contains(line, f.run.TestAlias) && strings.Contains(line, f.execution.Id) {
				found = true
			}
		}
		if !found {
			t.Errorf("group lacks its execution/test identity: %s\n%s", f.run.TestAlias, summary.String())
		}
	}
	for _, measured := range []string{"10 (1/2 outcomes reported)", "20 (1/2 outcomes reported)", "0 (1/2 outcomes reported)"} {
		if !strings.Contains(summary.String(), measured) {
			t.Errorf("summary omitted partial measurement %q\n%s", measured, summary.String())
		}
	}
	var results bytes.Buffer
	if err := Report(context.Background(), capture, "results", "text", &results); err != nil {
		t.Fatal(err)
	}
	for _, detail := range []string{`PASS expected "Capabilities", got "Capabilities"`, "FAIL actual 1, maximum 0", "PASS actual 1, maximum 2", "PASS actual 0, maximum 0", "4000", "10 (1/2 outcomes reported)"} {
		if !strings.Contains(results.String(), detail) {
			t.Errorf("results omitted %q\n%s", detail, results.String())
		}
	}
	for _, format := range []string{"json", "jsonl"} {
		var output bytes.Buffer
		if err := Report(context.Background(), capture, "summary", format, &output); err != nil {
			t.Fatal(err)
		}
		groups := []map[string]interface{}{}
		if format == "json" {
			var document struct {
				Groups []map[string]interface{} `json:"groups"`
			}
			if err := json.Unmarshal(output.Bytes(), &document); err != nil {
				t.Fatal(err)
			}
			groups = document.Groups
		} else {
			decoder := json.NewDecoder(&output)
			for {
				var record struct {
					Type string                 `json:"type"`
					Data map[string]interface{} `json:"data"`
				}
				if err := decoder.Decode(&record); err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					t.Fatal(err)
				}
				if record.Type == "group" {
					groups = append(groups, record.Data)
				}
			}
		}
		if len(groups) != 2 {
			t.Fatalf("%s group count = %d", format, len(groups))
		}
		for _, group := range groups {
			for _, metric := range []string{"input_tokens", "output_tokens", "reasoning_tokens", "cached_input_tokens", "total_tokens", "request_ms"} {
				if group[metric+"_observations"] != float64(1) || group["provider_outcomes"] != float64(2) {
					t.Errorf("%s lost grouped %s coverage: %v", format, metric, group)
				}
			}
		}
	}
}
