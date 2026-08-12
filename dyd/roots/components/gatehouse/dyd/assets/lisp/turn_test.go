package lisp

import (
	"strings"
	"testing"
)

func TestRunTurnPreloadsAuthorizedToolsAndResources(t *testing.T) {
	err, result := RunTurn(
		"(bytes/utf8/decode (example/read resource/endpoint))",
		[]TurnTool{{ID: "example", Source: "(list (pair 'read (fn (value) value)))"}},
		[]TurnResource{{ID: "endpoint", Value: []byte("https://example.test")}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.String(), `"https://example.test"`; got != want {
		t.Fatalf("RunTurn() = %s, want %s", got, want)
	}
}

func TestRunTurnKeepsSecretsTainted(t *testing.T) {
	err, result := RunTurn("resource/token", nil, []TurnResource{{ID: "token", Value: []byte("secret"), Secret: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !hasSecret(result) || result.String() != "#<secret>" {
		t.Fatalf("RunTurn() = %s, want secret result", result)
	}
}

func TestRunTurnBlocksDynamicImports(t *testing.T) {
	err, _ := RunTurn("(import (random @native:random/v1) (random/int 1))", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "unavailable during turn evaluation") {
		t.Fatalf("RunTurn() dynamic import error = %v", err)
	}
}
