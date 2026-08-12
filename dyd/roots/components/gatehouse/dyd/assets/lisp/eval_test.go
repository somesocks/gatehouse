package lisp

import "testing"

func TestEvalLisp(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{source: "(eval '(+ 1 2))", want: "3"},
		{source: "(let ((value 41)) (eval '(+ value 1)))", want: "42"},
		{source: "(secret? (eval (secret/mark '(+ 1 2))))", want: "#t"},
		{source: "(error? (error/catch (eval)))", want: "#t"},
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
