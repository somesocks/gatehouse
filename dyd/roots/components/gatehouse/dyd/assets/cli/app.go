package cli

import (
	"fmt"
	"os"

	clib "gatehouse/cli-builder"
)

func BuildCLI(version string, fingerprint string) clib.App {
	return clib.New("Gatehouse - a durable agent broker").
		WithCommand(versionCommand(version, fingerprint)).
		WithOption(clib.NewOption("help", "display help text for this command").WithType(clib.OptionTypeBool)).
		WithAction(func(clib.ActionRequest) int {
			fmt.Fprintln(os.Stdout, "Hello, Gatehouse!")
			return 0
		})
}
