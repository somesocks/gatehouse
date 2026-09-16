package lisp

import "testing"

func TestEvalLisp(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{source: "(eval '(+ 1 2))", want: "3"},
		{source: "(let ((value 41)) (eval '(+ value 1)))", want: "42"},
		{source: "(taint/secret? (eval (taint/secret/mark '(+ 1 2))))", want: "#t"},
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

func TestLetTailRecursion(t *testing.T) {
	err, result := Run(`(let ((count-down (fn (value)
  null
  (if (= value 0)
    0
    (let ((next (- value 1)))
      (count-down next))))))
  (count-down 100000))`)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "0" {
		t.Fatalf("tail recursion = %s, want 0", got)
	}
}

func TestLetEvaluatesMultipleBodies(t *testing.T) {
	err, result := Run(`(let ((value 41))
  (assert (= value 41))
  (+ value 1))`)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "42" {
		t.Fatalf("let result = %s, want 42", got)
	}

	err, _ = Run(`(let () (error/throw "first body") 42)`)
	if err == nil {
		t.Fatal("let evaluated its final body after an earlier body failed")
	}
}

func TestFunctionEvaluatesMultipleBodies(t *testing.T) {
	err, result := Run(`((fn (value)
  (assert (= value 41))
  (+ value 1)) 41)`)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "42" {
		t.Fatalf("function result = %s, want 42", got)
	}

	err, result = Run(`(list/map (fn (value) (assert (> value 0)) (+ value 1)) (list 1 2))`)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "(2 3)" {
		t.Fatalf("function map result = %s, want (2 3)", got)
	}

	err, _ = Run(`((fn () (error/throw "first body") 42))`)
	if err == nil {
		t.Fatal("function evaluated its final body after an earlier body failed")
	}
}

func TestIfsEvaluatesMatchingClause(t *testing.T) {
	err, result := Run(`(ifs
  ((= 1 2) missing)
  ((= 2 2) (assert #t) 7))`)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "7" {
		t.Fatalf("ifs result = %s, want 7", got)
	}

	err, result = Run(`(ifs ((= 1 2) missing) (#t 9))`)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "9" {
		t.Fatalf("ifs fallback result = %s, want 9", got)
	}

	err, _ = Run(`(ifs ((= 1 2) 7))`)
	if err == nil {
		t.Fatal("ifs returned without a matching clause")
	}

	err, _ = Run(`(ifs (1 2))`)
	if err == nil {
		t.Fatal("ifs accepted a non-Boolean condition")
	}
}

func TestIfsTailRecursion(t *testing.T) {
	err, result := Run(`(let ((count-down (fn (value)
  (ifs
    ((= value 0) 0)
    (#t (count-down (- value 1)))))))
  (count-down 100000))`)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.String(); got != "0" {
		t.Fatalf("ifs tail recursion = %s, want 0", got)
	}
}

func TestAssociationListLookup(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{source: "(list/assoc/get 'name (list (pair 'id 7) (pair 'name \"report.txt\")))", want: "\"report.txt\""},
		{source: "(list/assoc/get 'missing (list (pair 'id 7)))", want: "null"},
		{source: "(list/assoc/require 'name (list (pair 'name \"report.txt\")))", want: "\"report.txt\""},
		{source: "(taint/secret? (list/assoc/get 'name (taint/secret/mark (list (pair 'name \"report.txt\")))))", want: "#t"},
	} {
		err, result := Run(test.source)
		if err != nil {
			t.Fatalf("Run(%s): %v", test.source, err)
		}
		if got := result.String(); got != test.want {
			t.Fatalf("Run(%s) = %s, want %s", test.source, got, test.want)
		}
	}

	for _, source := range []string{
		"(list/assoc/get 'name (list 1))",
		"(list/assoc/require 'name null)",
	} {
		err, _ := Run(source)
		if err == nil {
			t.Fatalf("Run(%s) succeeded, want an error", source)
		}
	}
}

func TestListFirstAndLast(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{source: "(list/first (list 1 2 3))", want: "1"},
		{source: "(list/rest (list 1 2 3))", want: "(2 3)"},
		{source: "(list/last (list 1 2 3))", want: "3"},
		{source: "(list/first null)", want: "null"},
		{source: "(list/rest null)", want: "null"},
		{source: "(list/last null)", want: "null"},
		{source: "(taint/secret? (list/first (taint/secret/mark (list 1))))", want: "#t"},
		{source: "(taint/secret? (list/rest (taint/secret/mark (list 1))))", want: "#t"},
		{source: "(taint/secret? (list/last (taint/secret/mark (list 1))))", want: "#t"},
	} {
		err, result := Run(test.source)
		if err != nil {
			t.Fatalf("Run(%s): %v", test.source, err)
		}
		if got := result.String(); got != test.want {
			t.Fatalf("Run(%s) = %s, want %s", test.source, got, test.want)
		}
	}

	for _, source := range []string{
		"(list/first (pair 1 2))",
		"(list/rest (pair 1 2))",
		"(list/last (pair 1 2))",
	} {
		err, _ := Run(source)
		if err == nil {
			t.Fatalf("Run(%s) succeeded, want an error", source)
		}
	}
}
