package lisp

import "testing"

func TestFnApply(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{source: "(fn/apply + (list 1 2 3))", want: "6"},
		{source: "(fn/apply (fn (left right) (- left right)) (list 7 2))", want: "5"},
		{source: "(fn/apply (fn (first . rest) (list first rest)) (list 1 2 3))", want: "(1 (2 3))"},
		{source: "(fn/apply let '(((value 41)) (+ value 1)))", want: "42"},
		{source: "(error? (error/catch (fn/apply + 1)))", want: "#t"},
		{source: "(error? (error/catch (fn/apply + (pair 1 2))))", want: "#t"},
		{source: "(fn/apply if (list #f '(error/throw \"unused\") 42))", want: "42"},
		{source: "(fn/apply begin '((assert #t) (+ 1 2)))", want: "3"},
		{source: "(let ((offset 1)) ((fn/apply fn '((value) (+ value offset))) 41))", want: "42"},
		{source: "(fn/apply and (list #f '(error/throw \"unused\")))", want: "#f"},
		{source: "(fn/apply or (list #t '(error/throw \"unused\")))", want: "#t"},
		{source: "(fn/apply quote (list '(+ 1 2)))", want: "(+ 1 2)"},
		{source: "(taint/secret? (fn/apply list (taint/secret/mark (list 1))))", want: "#t"},
	} {
		err, result := Run(test.source)
		if err != nil {
			t.Fatalf("Run(%s): %v", test.source, err)
		}
		if got := result.String(); got != test.want {
			t.Fatalf("Run(%s) = %s, want %s", test.source, got, test.want)
		}
	}
}
