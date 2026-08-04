package lisp

import (
	_ "embed"
	"testing"
)

//go:embed help_test.lisp
var helpTests string

func TestHelpLisp(t *testing.T) {
	err, result := Run(helpTests)
	if err != nil {
		t.Fatal(err)
	}
	if result.kind != exprNull {
		t.Fatalf("Help test result = %s, want null", result)
	}
	if got, want := result.help, "Root leading help.\nRoot trailing help."; got != want {
		t.Fatalf("root help = %q, want %q", got, want)
	}
}
