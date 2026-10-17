package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBenchmarkRunRejectsInvalidOptionsWithoutStdout(t *testing.T) {
	for _, options := range [][]string{
		nil,
		{"--config="},
		{"--config", "missing.yaml"},
		{"--config=missing.yaml", "--report=unknown"},
		{"--config=missing.yaml", "--format=unknown"},
		{"--config=missing.yaml", "--format=sqlite"},
		{"--config=missing.yaml", "--output="},
		{"--config=missing.yaml", "--unknown=value"},
	} {
		var stdout, stderr bytes.Buffer
		arguments := append([]string{"gatehouse", "benchmark", "run"}, options...)
		code := BuildCLI("test", "build").RunWithWriters(arguments, &stdout, &stderr)
		if code == 0 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("arguments=%v code=%d stdout=%q stderr=%q", options, code, stdout.String(), stderr.String())
		}
	}
}

func TestBenchmarkRunCLIReportsAndGrades(t *testing.T) {
	for _, expected := range []string{"24", "25"} {
		t.Run(expected, func(t *testing.T) {
			path := benchmarkCLIConfig(t, expected)
			var stdout, stderr bytes.Buffer
			arguments := []string{"gatehouse", "benchmark", "run", "--config=" + path, "--report=results", "--format=json"}
			if expected == "24" {
				arguments = append(arguments, "--output=-")
			}
			code := BuildCLI("test", "build").RunWithWriters(arguments, &stdout, &stderr)
			want := 0
			if expected != "24" {
				want = 1
			}
			if code != want {
				t.Fatalf("code=%d, want=%d, stderr=%s", code, want, stderr.String())
			}
			var report map[string]interface{}
			if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
				t.Fatalf("invalid stdout: %v\n%s", err, stdout.String())
			}
			if report["report"] != "results" || len(report["results"].([]interface{})) != 1 {
				t.Fatalf("unexpected report: %v", report)
			}
			if !strings.Contains(stderr.String(), "benchmark") {
				t.Fatal("progress was not routed to stderr")
			}
		})
	}
}

func benchmarkCLIConfig(t *testing.T, expected string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tests.yaml")
	text := `api_version: v1
agent_providers:
  - {alias: builtin, protocol: builtin}
agent_models:
  - {alias: assistant, provider: builtin, model: dummy.fixed-reply, parameters: {text: "24"}}
system_prompts:
  - {alias: baseline, mode: builtin}
questions:
  - {alias: question, text: "temperature?", expected: {output: "EXPECTED", max_tool_calls: 0}}
lisp_environments:
  - {alias: session}
tests:
  - alias: example
    matrix: {models: [assistant], system_prompts: [baseline], questions: [question], lisp_environments: [session]}
`
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(text, "EXPECTED", expected)), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBenchmarkRunFileOutput(t *testing.T) {
	for _, test := range []struct{ expected, format string }{{"24", "json"}, {"25", "json"}, {"24", "sqlite"}} {
		t.Run(test.expected+"/"+test.format, func(t *testing.T) {
			path := benchmarkCLIConfig(t, test.expected)
			output := filepath.Join(t.TempDir(), "results."+test.format)
			if err := os.WriteFile(output, bytes.Repeat([]byte("old content\n"), 2048), 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			code := BuildCLI("test", "build").RunWithWriters([]string{"gatehouse", "benchmark", "run", "--config=" + path, "--report=full", "--format=" + test.format, "--output=" + output}, &stdout, &stderr)
			want := 0
			wantGrade := "passed"
			if test.expected != "24" {
				want = 1
				wantGrade = "failed"
			}
			if code != want || stdout.Len() != 0 {
				t.Fatalf("code=%d want=%d stdout=%q stderr=%s", code, want, stdout.String(), stderr.String())
			}
			if test.format == "sqlite" {
				db, err := sql.Open("sqlite", output)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				var grade string
				if err := db.QueryRow(`SELECT grade FROM run_results`).Scan(&grade); err != nil || grade != "passed" {
					t.Fatalf("saved grade=%q err=%v", grade, err)
				}
			} else {
				data, err := os.ReadFile(output)
				if err != nil {
					t.Fatal(err)
				}
				var report map[string]interface{}
				if err := json.Unmarshal(data, &report); err != nil {
					t.Fatalf("invalid saved report: %v", err)
				}
				grade := report["results"].([]interface{})[0].(map[string]interface{})["grade"]
				if grade != wantGrade {
					t.Fatalf("saved grade=%v, exit=%d", grade, code)
				}
			}
		})
	}
}

func TestBenchmarkRunRejectsUnavailableOutputBeforeExecution(t *testing.T) {
	path := benchmarkCLIConfig(t, "24")
	output := filepath.Join(t.TempDir(), "missing-directory", "results.json")
	var stdout, stderr bytes.Buffer
	code := BuildCLI("test", "build").RunWithWriters([]string{"gatehouse", "benchmark", "run", "--config=" + path, "--output=" + output}, &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "open benchmark output:") || strings.Contains(stderr.String(), "benchmark brn_") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
