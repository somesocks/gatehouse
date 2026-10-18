package lisp

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestImportMultipleBodies(t *testing.T) {
	const one = `(@native:fixture/one/v1 (list (pair 'value 7)))`
	const two = `(@native:fixture/two/v1 (list (pair 'value 11)))`
	const allowlist = `(@native:fixture/one/v1 @native:fixture/two/v1)`
	for _, importer := range []struct{ name, wrapper string }{
		{"native", `%s`},
		{"defined", `(import/define ` + one + ` ` + two + ` %s)`},
		{"defined then native", `(import/define ` + one + ` %s)`},
		{"native then defined", `(import/define ` + two + ` %s)`},
		{"nested definitions", `(import/define ` + one + ` (import/define ` + two + ` %s))`},
		{"restricted native", `(import/restrict ` + allowlist + ` %s)`},
		{"restricted defined", `(import/define ` + one + ` ` + two + ` (import/restrict ` + allowlist + ` %s))`},
		{"defined restricted", `(import/restrict ` + allowlist + ` (import/define ` + one + ` %s))`},
		{"shadowed begin", `(let ((begin null)) %s)`},
	} {
		t.Run(importer.name, func(t *testing.T) {
			for _, test := range []struct {
				name, body, want string
				calls            []int64
			}{
				{"ordered last value", `(record one/value) (record two/value) (int/add one/value two/value)`, "18", []int64{7, 11}},
				{"stop on exception", `(record one/value) (error/throw (list 'stop two/value)) (record 99)`, "(stop 11)", []int64{7}},
				{"captured scope", `(record one/value) (fn () (int/add one/value two/value))`, "(18 null null)", []int64{7}},
				{"discarded taint", `(taint/secret/mark one/value) (int/add one/value two/value)`, "18", nil},
			} {
				t.Run(test.name, func(t *testing.T) {
					var calls []int64
					source := `(import (one @native:fixture/one/v1) (two @native:fixture/two/v1) ` + test.body + `)`
					source = fmt.Sprintf(importer.wrapper, source)
					switch test.name {
					case "stop on exception":
						source = `(error/value (error/catch ` + source + `))`
					case "captured scope":
						source = `(let ((read ` + source + `)) (list (read) (help/env "one/") (help/env "two/")))`
					}
					err, result := Evaluate(source, EvalOptions{
						HostModules: []HostModule{
							{ID: "native:fixture/one/v1", Exports: []HostExport{{Name: "value", Value: Integer(7)}}},
							{ID: "native:fixture/two/v1", Exports: []HostExport{{Name: "value", Value: Integer(11)}}},
						},
						Bindings: []Binding{{Name: "record", Value: Function(func(arguments []Expr) (error, Expr) {
							err, value := RequireInteger(arguments[0])
							if err != nil {
								return err, nil
							}
							calls = append(calls, value)
							return nil, Integer(value)
						})}},
					})
					if err != nil || result.String() != test.want || TaintOf(result) != TaintNone {
						t.Fatalf("Evaluate() = (%v, %v), want %s", result, err, test.want)
					}
					if !reflect.DeepEqual(calls, test.calls) {
						t.Fatalf("calls = %v, want %v", calls, test.calls)
					}
				})
			}
		})
	}
}

func TestImportBodyStopsDeclarationParsing(t *testing.T) {
	calls := 0
	err, result := Evaluate(`(import/restrict (@native:fixture/one/v1)
  (import (one @native:fixture/one/v1)
    null
    (inspect @native:fixture/two/v1)
    one/value))`, EvalOptions{
		HostModules: []HostModule{{ID: "native:fixture/one/v1", Exports: []HostExport{{Name: "value", Value: Integer(7)}}}},
		Bindings: []Binding{{Name: "inspect", Value: Function(func(arguments []Expr) (error, Expr) {
			calls++
			if len(arguments) != 1 || arguments[0].String() != "@native:fixture/two/v1" {
				return Errorf("unexpected reference arguments"), nil
			}
			return nil, Null()
		})}},
	})
	if err != nil || result.String() != "7" || calls != 1 {
		t.Fatalf("Evaluate() = (%v, %v), calls = %d; want 7 and one body call", result, err, calls)
	}
}

func TestImportMultipleBodyValidation(t *testing.T) {
	for _, test := range []struct{ name, source, contains string }{
		{"no arguments", `(import)`, "requires declarations and a body expression"},
		{"one declaration without body", `(import (json @native:json/v1))`, "requires declarations and a body expression"},
		{"two declarations without body", `(import (json @native:json/v1) (time @native:time/v1))`, "requires declarations and a body expression"},
		{"malformed declaration", `(import (json 1) null null)`, "must contain a name and module reference"},
		{"duplicate alias", `(import (json @native:json/v1) (json @native:time/v1) null null)`, `alias "json" is duplicated`},
		{"restricted declaration", `(import/restrict (@native:json/v1) (import (json @native:json/v1) (time @native:time/v1) (error/throw "body ran") null))`, "is not allowed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err, _ := Run(test.source)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run() error = %v, want %q", err, test.contains)
			}
		})
	}
}

func TestImportMultipleBodiesThroughApply(t *testing.T) {
	err, result := Run(`(fn/apply import
  (list (list 'json @native:json/v1)
    '(assert (= (json/encode (json/null)) "null"))
    '(json/encode (json/number "42"))))`)
	if err != nil || result.String() != `"42"` {
		t.Fatalf("Run() = (%v, %v), want string 42", result, err)
	}
}
