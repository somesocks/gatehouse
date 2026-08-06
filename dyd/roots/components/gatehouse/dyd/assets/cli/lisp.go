package cli

import (
	"fmt"
	"io"
	"os"

	clib "gatehouse/cli-builder"
	"gatehouse/lisp"
)

var lispCommand = clib.NewCommand("lisp", "evaluate Lisp source").
	WithAction(func(request clib.ActionRequest) int {
		if err := request.App.Usage(request.Invocation, os.Stdout); err != nil {
			return 1
		}
		return 0
	}).
	WithCommand(
		clib.NewCommand("eval", "evaluate one Lisp expression").
			WithArg(clib.NewArg("source", "Lisp source to evaluate").AsOptional()).
			WithOption(clib.NewOption("from-stdin", "read Lisp source from standard input").WithType(clib.OptionTypeBool)).
			WithAction(func(request clib.ActionRequest) int {
				fromStdin, _ := request.Opts["from-stdin"].(bool)
				var source string
				switch {
				case fromStdin && len(request.Args) != 0:
					fmt.Fprintln(os.Stderr, "lisp eval: source argument cannot be used with --from-stdin")
					return 1
				case fromStdin:
					input, err := io.ReadAll(os.Stdin)
					if err != nil {
						fmt.Fprintf(os.Stderr, "read Lisp source: %v\n", err)
						return 1
					}
					source = string(input)
				case len(request.Args) != 1:
					fmt.Fprintln(os.Stderr, "lisp eval requires a source argument or --from-stdin")
					return 1
				default:
					source = request.Args[0]
				}

				err, result := lisp.Run(source)
				if err != nil {
					fmt.Fprintf(os.Stderr, "evaluate Lisp: %v\n", err)
					return 1
				}
				fmt.Fprintln(os.Stdout, result.String())
				return 0
			}),
	)
