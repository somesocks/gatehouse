package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"gatehouse/benchmark"
	clib "gatehouse/cli-builder"
	"gatehouse/diagnostics"
)

func benchmarkCommand(version, fingerprint string) clib.Command {
	return clib.NewCommand("benchmark", "manage Gatehouse benchmark matrices").
		WithAction(func(request clib.ActionRequest) int {
			if err := request.App.Usage(request.Invocation, os.Stdout); err != nil {
				return 1
			}
			return 0
		}).
		WithCommand(
			clib.NewCommand("validate", "validate a YAML benchmark configuration and count its runs").
				WithArg(clib.NewArg("path", "path to tests.yaml")).
				WithAction(func(request clib.ActionRequest) int {
					path := request.Args[0]
					err, configuration := benchmark.ValidateFile(path)
					if err != nil {
						fmt.Fprintf(os.Stderr, "benchmark configuration is invalid: %s: %v\n", path, err)
						return 1
					}
					fmt.Fprintf(os.Stderr, "benchmark configuration is valid: %s (%d tests, %d runs)\n", path, len(configuration.Document.Tests), configuration.RunCount)
					return 0
				}),
		).
		WithCommand(clib.NewCommand("run", "execute and report a benchmark matrix").
			WithOption(clib.NewOption("config", "required path to the benchmark YAML configuration")).
			WithOption(clib.NewOption("report", "summary (default), results, or full")).
			WithOption(clib.NewOption("format", "text (default), json, jsonl, or sqlite (full only)")).
			WithOption(clib.NewOption("output", "report file path, or - for stdout (default)")).
			WithOption(clib.NewOption("parallel", "maximum concurrent runs (default 8); use 1 for serial execution")).
			WithAction(func(request clib.ActionRequest) (code int) {
				var signalCode atomic.Int32
				// Registered first so this runs after report/file cleanup. An
				// interrupted execution retains its signal status even if output fails.
				defer func() {
					if received := signalCode.Load(); received != 0 {
						code = int(received)
					}
				}()
				stdout, stderr := request.Stdout, request.Stderr
				if stdout == nil {
					stdout = os.Stdout
				}
				if stderr == nil {
					stderr = os.Stderr
				}
				path, _ := request.Opts["config"].(string)
				if strings.TrimSpace(path) == "" {
					fmt.Fprintln(stderr, "benchmark run requires --config=<path>")
					return 1
				}
				parallel := benchmark.DefaultParallel
				if value, ok := request.Opts["parallel"].(string); ok {
					parsed, err := strconv.Atoi(value)
					if err != nil || parsed < 1 {
						fmt.Fprintln(stderr, "benchmark --parallel must be a positive integer")
						return 1
					}
					parallel = parsed
				}
				report, format := "summary", "text"
				if value, ok := request.Opts["report"].(string); ok {
					report = value
				}
				if value, ok := request.Opts["format"].(string); ok {
					format = value
				}
				if err := benchmark.ValidateOutput(report, format); err != nil {
					fmt.Fprintln(stderr, err)
					return 1
				}
				output := "-"
				if value, ok := request.Opts["output"].(string); ok {
					output = value
				}
				if output == "" {
					fmt.Fprintln(stderr, "benchmark output path must not be empty; use --output=- for stdout")
					return 1
				}
				err, configuration := benchmark.ValidateFile(path)
				if err != nil {
					fmt.Fprintf(stderr, "benchmark configuration is invalid: %v\n", err)
					return 1
				}
				reportOutput := stdout
				if output != "-" {
					file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
					if err != nil {
						fmt.Fprintf(stderr, "open benchmark output: %v\n", err)
						return 1
					}
					defer func() {
						if err := file.Close(); err != nil {
							fmt.Fprintf(stderr, "close benchmark output: %v\n", err)
							code = 1
						}
					}()
					reportOutput = file
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				configuredSignals := append([]os.Signal{os.Interrupt, syscall.SIGTERM}, diagnostics.Signals()...)
				signals := make(chan os.Signal, len(configuredSignals))
				signal.Notify(signals, configuredSignals...)
				done, stopped := make(chan struct{}), make(chan struct{})
				defer func() {
					signal.Stop(signals)
					close(done)
					<-stopped
				}()
				go func() {
					defer close(stopped)
					for {
						select {
						case received := <-signals:
							if received == os.Interrupt || received == syscall.SIGTERM {
								code := int32(130)
								if received == syscall.SIGTERM {
									code = 143
								}
								signalCode.CompareAndSwap(0, code)
								cancel()
							} else if err := diagnostics.EmitSignal(received, stdout, stderr); err != nil {
								fmt.Fprintf(stderr, "emit diagnostic signal: %v\n", err)
							}
						case <-done:
							return
						}
					}
				}()
				executionErr, capture := benchmark.ExecuteWithOptions(ctx, configuration, path, version, fingerprint, stderr, benchmark.ExecutionOptions{Parallel: parallel})
				if capture == nil {
					fmt.Fprintf(stderr, "benchmark execution failed: %v\n", executionErr)
					return 1
				}
				defer capture.Close()
				cleanup, finish := context.WithTimeout(context.Background(), time.Minute)
				defer finish()
				if executionErr != nil {
					fmt.Fprintf(stderr, "benchmark execution: %v\n", executionErr)
				}
				if err := benchmark.Report(cleanup, capture, report, format, reportOutput); err != nil {
					fmt.Fprintf(stderr, "write benchmark report: %v\n", err)
					return 1
				}
				failed, err := benchmark.ResultsFailed(cleanup, capture)
				if err != nil {
					fmt.Fprintf(stderr, "read benchmark grades: %v\n", err)
					return 1
				}
				if executionErr != nil || failed {
					return 1
				}
				return 0
			}))
}
