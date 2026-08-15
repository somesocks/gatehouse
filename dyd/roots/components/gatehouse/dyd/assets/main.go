package main

import (
	"os"

	"gatehouse/cli"
	"gatehouse/diagnostics"
)

var Version string
var Fingerprint string

func main() {
	if err := diagnostics.SetupFromEnv(); err != nil {
		_, _ = os.Stderr.WriteString("error initializing diagnostics: " + err.Error() + "\n")
		os.Exit(2)
	}
	args := os.Args
	args[0] = "gatehouse"
	exitCode := cli.BuildCLI(Version, Fingerprint).Run(args, os.Stdout)
	if err := diagnostics.EmitProcessExit(os.Stdout, os.Stderr); err != nil {
		_, _ = os.Stderr.WriteString("error emitting process-exit diagnostics: " + err.Error() + "\n")
	}
	os.Exit(exitCode)
}
