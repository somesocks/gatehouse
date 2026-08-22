package lisp

import (
	_ "embed"
	"strings"
	"testing"
	"time"
)

//go:embed error_test.lisp
var errorTests string

func TestErrorLisp(t *testing.T) {
	err, result := Run(errorTests)
	if err != nil {
		t.Fatal(err)
	}
	if !isNullValue(result) {
		t.Fatalf("Error test result = %s, want null", result)
	}
}

func TestAssertTimingReportsOverrun(t *testing.T) {
	err, result := Evaluate(`(error/value (error/catch (assert/timing 5 (pause))))`, EvalOptions{
		Bindings: []Binding{{Name: "pause", Value: Function(func([]Expr) (error, Expr) {
			time.Sleep(20 * time.Millisecond)
			return nil, Null()
		})}},
	})
	if err != nil || !strings.Contains(result.String(), "assert/timing exceeded 5 milliseconds") {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}
