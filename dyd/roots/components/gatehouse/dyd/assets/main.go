package main

import (
	"os"

	"gatehouse/cli"
)

var Version string
var Fingerprint string

func main() {
	args := os.Args
	args[0] = "gatehouse"
	os.Exit(cli.BuildCLI(Version, Fingerprint).Run(args, os.Stdout))
}
