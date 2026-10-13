package cli

import (
	"fmt"
	"os"

	"gatehouse/benchmark"
	clib "gatehouse/cli-builder"
)

var benchmarkCommand = clib.NewCommand("benchmark", "manage Gatehouse benchmark matrices").
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
	)
