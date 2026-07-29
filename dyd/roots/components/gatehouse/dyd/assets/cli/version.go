package cli

import (
	"fmt"
	"runtime"

	clib "gatehouse/cli-builder"
)

func versionCommand(version string, fingerprint string) clib.Command {
	return clib.NewCommand("version", "print detailed version information").
		WithAction(func(clib.ActionRequest) int {
			fmt.Println("version=" + version)
			fmt.Println("source_fingerprint=" + fingerprint)
			fmt.Println("arch=" + runtime.GOARCH)
			fmt.Println("os=" + runtime.GOOS)
			return 0
		})
}
