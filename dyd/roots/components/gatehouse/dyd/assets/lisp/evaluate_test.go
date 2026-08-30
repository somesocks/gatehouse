package lisp

import (
	"strings"
	"testing"
)

func TestEvaluateBindsValuesAndSourceModuleExports(t *testing.T) {
	err, result := Evaluate("(bytes/utf8/decode (example/read endpoint))", EvalOptions{
		Bindings:      []Binding{{Name: "endpoint", Value: Bytes([]byte("https://example.test"))}},
		SourceModules: []SourceModule{{ID: "example", Source: "(list (pair 'read (fn (value) value)))"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.String(), `"https://example.test"`; got != want {
		t.Fatalf("Evaluate() = %s, want %s", got, want)
	}
}

func TestEvaluateKeepsSecretsTainted(t *testing.T) {
	err, result := Evaluate("token", EvalOptions{Bindings: []Binding{{Name: "token", Value: MarkSecret(String("secret"))}}})
	if err != nil {
		t.Fatal(err)
	}
	if !IsSecret(result) || result.String() != "#<secret>" {
		t.Fatalf("Evaluate() = %s, want secret result", result)
	}
}

func TestEvaluatePreludeEvaluatesParsedProgramLexically(t *testing.T) {
	err, result := Evaluate("(+ factor 1)", EvalOptions{Prelude: "(let ((factor 41)) (eval program))"})
	if err != nil || result.String() != "42" {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestEvaluateHostModulesCanBeImported(t *testing.T) {
	err, result := Evaluate(`(import (host @native:example/v1) (host/read "ok"))`, EvalOptions{
		HostModules: []HostModule{{ID: "native:example/v1", Exports: []HostExport{{Name: "read", Value: Function(func(arguments []Expr) (error, Expr) {
			if len(arguments) != 1 {
				return Errorf("read requires one argument"), nil
			}
			err, value := RequireString(arguments[0])
			if err != nil {
				return err, nil
			}
			return nil, String(value)
		})}}}},
	})
	if err != nil || result.String() != `"ok"` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestEvaluateSourceModulesCanImportHostModules(t *testing.T) {
	err, result := Evaluate("example/value", EvalOptions{
		HostModules:   []HostModule{{ID: "native:value/v1", Exports: []HostExport{{Name: "value", Value: Integer(41)}}}},
		SourceModules: []SourceModule{{ID: "example", Source: "(import (value @native:value/v1) (list (pair 'value (+ value/value 1))))"}},
	})
	if err != nil || result.String() != "42" {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestEvaluateRejectsInvalidAndDuplicateInput(t *testing.T) {
	tests := []struct {
		name    string
		options EvalOptions
		contains string
	}{
		{name: "binding", options: EvalOptions{Bindings: []Binding{{}}}, contains: "binding is invalid"},
		{name: "binding duplicate", options: EvalOptions{Bindings: []Binding{{Name: "value", Value: Null()}, {Name: "value", Value: Null()}}}, contains: `binding "value" is duplicated`},
		{name: "host module", options: EvalOptions{HostModules: []HostModule{{}}}, contains: "host module ID must not be blank"},
		{name: "host export", options: EvalOptions{HostModules: []HostModule{{ID: "native:example/v1", Exports: []HostExport{{}}}}}, contains: `host module "native:example/v1" has an invalid export`},
		{name: "host export duplicate", options: EvalOptions{HostModules: []HostModule{{ID: "native:example/v1", Exports: []HostExport{{Name: "value", Value: Null()}, {Name: "value", Value: Null()}}}}}, contains: `host module "native:example/v1" export "value" is duplicated`},
		{name: "source module", options: EvalOptions{SourceModules: []SourceModule{{}}}, contains: "source module ID must not be blank"},
		{name: "source module duplicate", options: EvalOptions{SourceModules: []SourceModule{{ID: "example", Source: "null"}, {ID: "example"}}}, contains: `source module "example" is duplicated`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err, _ := Evaluate("null", test.options)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Evaluate() error = %v, want %q", err, test.contains)
			}
		})
	}
}
