package lisp

import "testing"

func TestFnBind(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{source: "((fn/bind +) 1 2 3)", want: "6"},
		{source: "((fn/bind + 1 2) 3)", want: "6"},
		{source: "((fn/bind (fn (first second third) (- first second third)) 10) 3 2)", want: "5"},
		{source: "(let ((value 2) (add (fn/bind + value))) (add 3))", want: "5"},
		{source: "(secret? ((fn/bind list (secret/mark 1)) 2))", want: "#t"},
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
