package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"gatehouse/diagnostics"
)

type rejectedReportWriter struct{}

func (rejectedReportWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// Run the command in a child process so signal tests never signal the test host.
func TestBenchmarkRunProcess(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_BENCHMARK_PROCESS") != "1" {
		return
	}
	if err := diagnostics.SetupFromEnv(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	for index, argument := range os.Args {
		if argument != "--" {
			continue
		}
		var output io.Writer = os.Stdout
		if os.Getenv("GATEHOUSE_TEST_REJECT_REPORT") == "1" {
			output = rejectedReportWriter{}
		}
		os.Exit(BuildCLI("test", "build").RunWithWriters(os.Args[index+1:], output, os.Stderr))
	}
	t.Fatal("missing child command arguments")
}

type benchmarkProcessLog struct {
	mu         sync.Mutex
	buffer     bytes.Buffer
	once       sync.Once
	diagnostic chan struct{}
}

func (log *benchmarkProcessLog) Write(data []byte) (int, error) {
	log.mu.Lock()
	defer log.mu.Unlock()
	n, err := log.buffer.Write(data)
	if bytes.Contains(log.buffer.Bytes(), []byte(`"goroutines"`)) {
		log.once.Do(func() { close(log.diagnostic) })
	}
	return n, err
}

func (log *benchmarkProcessLog) String() string {
	log.mu.Lock()
	defer log.mu.Unlock()
	return log.buffer.String()
}

func TestBenchmarkRunDiagnosticSignalsAndInterruptedOutput(t *testing.T) {
	for _, test := range []struct {
		name, diagnosticName, diagnosticOutput string
		diagnosticSignal, stopSignal           syscall.Signal
		exitCode                               int
		rejectReport                           bool
	}{
		{"stdout diagnostic and file report", "USR1", "stdout", syscall.SIGUSR1, syscall.SIGINT, 130, false},
		{"HUP diagnostic and failed SIGTERM report", "HUP", "stderr", syscall.SIGHUP, syscall.SIGTERM, 143, true},
		{"stderr diagnostic and failed SIGINT report", "USR1", "stderr", syscall.SIGUSR1, syscall.SIGINT, 130, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			started := make(chan struct{}, 1)
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				select {
				case started <- struct{}{}:
				default:
				}
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			defer server.Close()
			defer close(release)
			path := benchmarkCLIConfig(t, "24")
			configuration := fmt.Sprintf(`api_version: v1
agent_providers:
  - {alias: provider, protocol: openai-responses, endpoint: %q, api_key: {sources: ["env:BENCH_PROCESS_KEY"]}}
agent_models:
  - {alias: assistant, provider: provider, model: test-model, parameters: {}}
system_prompts:
  - {alias: baseline, mode: builtin}
questions:
  - {alias: question, text: hello}
lisp_environments:
  - {alias: session}
tests:
  - alias: example
    matrix: {models: [assistant], system_prompts: [baseline], questions: [question], lisp_environments: [session]}
`, server.URL)
			if err := os.WriteFile(path, []byte(configuration), 0o600); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(t.TempDir(), "results.json")
			arguments := []string{"-test.run=^TestBenchmarkRunProcess$", "--", "gatehouse", "benchmark", "run", "--config=" + path, "--report=full", "--format=json"}
			if !test.rejectReport {
				arguments = append(arguments, "--output="+output)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], arguments...)
			diagConfig := fmt.Sprintf(`json:{"version":2,"outputs":{"diagnostic":{"type":%q}},"rules":[],"triggers":[{"id":"snapshot","source":{"type":"signal","signal":%q},"actions":[{"type":"emit_runtime","output":"diagnostic"}]}]}`, test.diagnosticOutput, test.diagnosticName)
			command.Env = append(os.Environ(), "GATEHOUSE_TEST_BENCHMARK_PROCESS=1", "BENCH_PROCESS_KEY=test-key", "GATEHOUSE_DIAG="+diagConfig)
			if test.rejectReport {
				command.Env = append(command.Env, "GATEHOUSE_TEST_REJECT_REPORT=1")
			}
			stdout := &benchmarkProcessLog{diagnostic: make(chan struct{})}
			stderr := &benchmarkProcessLog{diagnostic: make(chan struct{})}
			command.Stdout, command.Stderr = stdout, stderr
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			finished := make(chan error, 1)
			exited := make(chan struct{})
			go func() { finished <- command.Wait(); close(exited) }()
			defer func() {
				select {
				case <-exited:
				default:
					_ = command.Process.Kill()
					<-exited
				}
			}()
			select {
			case <-started:
			case err := <-finished:
				t.Fatalf("command exited before provider request: %v\n%s", err, stderr.String())
			case <-ctx.Done():
				t.Fatalf("provider did not start: %s", stderr.String())
			}
			if err := command.Process.Signal(test.diagnosticSignal); err != nil {
				t.Fatal(err)
			}
			diagnostic := stderr.diagnostic
			if test.diagnosticOutput == "stdout" {
				diagnostic = stdout.diagnostic
			}
			select {
			case <-diagnostic:
			case err := <-finished:
				t.Fatalf("diagnostic signal terminated the command: %v\n%s", err, stderr.String())
			case <-ctx.Done():
				t.Fatalf("diagnostic signal produced no output: stdout=%s stderr=%s", stdout.String(), stderr.String())
			}
			if err := command.Process.Signal(test.stopSignal); err != nil {
				t.Fatal(err)
			}
			select {
			case <-finished:
			case <-ctx.Done():
				t.Fatalf("command did not finish after interruption: %s", stderr.String())
			}
			if code := command.ProcessState.ExitCode(); code != test.exitCode {
				t.Fatalf("exit=%d want=%d\n%s", code, test.exitCode, stderr.String())
			}
			if test.rejectReport {
				if !strings.Contains(stderr.String(), "write benchmark report:") {
					t.Fatalf("report failure was not reported: %s", stderr.String())
				}
			} else {
				data, err := os.ReadFile(output)
				if err != nil {
					t.Fatal(err)
				}
				var report map[string]interface{}
				if err := json.Unmarshal(data, &report); err != nil {
					t.Fatalf("partial report: %v\n%s", err, data)
				}
				if report["results"].([]interface{})[0].(map[string]interface{})["status"] != "cancelled" {
					t.Fatalf("partial report lost interruption status: %s", data)
				}
				if !json.Valid([]byte(stdout.String())) {
					t.Fatalf("stdout diagnostic was redirected or mixed into report: %s", stdout.String())
				}
			}
		})
	}
}
