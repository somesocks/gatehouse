package cli

import (
	"fmt"
	"os"

	clib "gatehouse/cli-builder"
	"gatehouse/config"
)

var configCommand = clib.NewCommand("config", "manage Gatehouse configuration").
	WithAction(func(request clib.ActionRequest) int {
		if err := request.App.Usage(request.Invocation, os.Stdout); err != nil {
			return 1
		}
		return 0
	}).
	WithCommand(
		clib.NewCommand("validate", "validate a YAML or JSON configuration file").
			WithArg(clib.NewArg("path", "path to the configuration file")).
			WithAction(func(request clib.ActionRequest) int {
				path := request.Args[0]
				document, err := config.ValidateFile(path)
				if err != nil {
					fmt.Fprintf(os.Stderr, "configuration is invalid: %s: %v\n", path, err)
					return 1
				}
				_, warnings, err := config.ResolveDatabase(document)
				if err != nil {
					fmt.Fprintf(os.Stderr, "configuration is invalid: %s: %v\n", path, err)
					return 1
				}
				for _, warning := range warnings {
					fmt.Fprintf(os.Stderr, "warning: %s\n", warning.Message)
				}

				fmt.Fprintf(os.Stderr, "configuration is valid: %s\n", path)
				return 0
			}),
	)
