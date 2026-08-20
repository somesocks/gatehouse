package lisp

import "testing"

func TestApply(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{source: "(apply + (list 1 2 3))", want: "6"},
		{source: "(apply (fn (left right) (- left right)) (list 7 2))", want: "5"},
		{source: "(apply (fn (first . rest) (list first rest)) (list 1 2 3))", want: "(1 (2 3))"},
		{source: "(apply let '(((value 41)) (+ value 1)))", want: "42"},
		{source: "(error? (error/catch (apply + 1)))", want: "#t"},
		{source: "(error? (error/catch (apply + (pair 1 2))))", want: "#t"},
		{source: "(apply if (list #f '(error/throw \"unused\") 42))", want: "42"},
		{source: "(apply begin '((assert #t) (+ 1 2)))", want: "3"},
		{source: "(let ((offset 1)) ((apply fn '((value) (+ value offset))) 41))", want: "42"},
		{source: "(secret? (apply list (secret/mark (list 1))))", want: "#t"},
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
