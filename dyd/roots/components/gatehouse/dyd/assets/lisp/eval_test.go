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
