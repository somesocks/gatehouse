package cli

import (
	"os"

	clib "gatehouse/cli-builder"
)

func BuildCLI(version string, fingerprint string) clib.App {
	return clib.New("Gatehouse - a durable agent broker").
		WithCommand(versionCommand(version, fingerprint)).
		WithOption(clib.NewOption("help", "display help text for this command").WithType(clib.OptionTypeBool)).
		WithAction(func(request clib.ActionRequest) int {
			if err := request.App.Usage(request.Invocation, os.Stdout); err != nil {
				return 1
			}
			return 0
		})
}
