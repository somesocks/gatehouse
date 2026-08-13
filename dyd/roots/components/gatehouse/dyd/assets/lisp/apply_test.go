package lisp

import "testing"

func TestApply(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{source: "(apply + (list 1 2 3))", want: "6"},
		{source: "(apply (fn (left right) (- left right)) (list 7 2))", want: "5"},
		{source: "(error? (error/catch (apply + 1)))", want: "#t"},
		{source: "(error? (error/catch (apply + (pair 1 2))))", want: "#t"},
		{source: "(error? (error/catch (apply if (list #t 1 2))))", want: "#t"},
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
