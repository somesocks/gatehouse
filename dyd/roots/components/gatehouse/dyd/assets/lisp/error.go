package lisp

import "fmt"

type Error struct {
	Span    Span
	Message string
}

func (err Error) Error() string {
	if err.Span.End > err.Span.Start {
		return fmt.Sprintf("%d:%d: %s", err.Span.Start, err.Span.End, err.Message)
	}
	return err.Message
}

func expressionError(span Span, format string, args ...any) error {
	return Error{Span: span, Message: fmt.Sprintf(format, args...)}
}

type errorValue struct {
	value Expr
}

type raisedError struct {
	value Expr
}

func (err *raisedError) Error() string {
	if err.value.secret {
		return "#<secret>"
	}
	return "thrown error: " + err.value.error.value.String()
}

func errorExpression(value Expr, span Span) Expr {
	return Expr{
		kind:   exprError,
		span:   span,
		secret: value.secret,
		error:  &errorValue{value: value},
	}
}

func catchError(evaluator *evaluator, env *environment, forms []Expr, span Span) (error, Expr) {
	if len(forms) != 1 {
		return expressionError(span, "error/catch requires one expression"), Expr{}
	}
	err, result := evaluator.eval(forms[0], env)
	if err == nil {
		return nil, result
	}
	return nil, caughtError(err, forms[0].span)
}

func throwError(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "error/throw requires one value"), Expr{}
	}
	value := arguments[0]
	if value.kind != exprError {
		value = errorExpression(value, span)
	}
	return &raisedError{value: value}, Expr{}
}

func assertValue(evaluator *evaluator, env *environment, forms []Expr, span Span) (error, Expr) {
	if len(forms) < 1 || len(forms) > 2 {
		return expressionError(span, "assert requires a condition and optional value"), Expr{}
	}
	err, condition := evaluator.eval(forms[0], env)
	if err != nil {
		return err, Expr{}
	}
	err, valid := requireBoolean(condition, forms[0].span)
	if err != nil {
		return err, Expr{}
	}
	if !valid {
		return &raisedError{value: errorExpression(stringValue("assertion failed", Span{}), span)}, Expr{}
	}
	if len(forms) == 1 {
		return nil, null(Span{})
	}
	return evaluator.eval(forms[1], env)
}

func isError(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "error? requires one argument"), Expr{}
	}
	return nil, boolean(arguments[0].kind == exprError, Span{})
}

func errorValueOf(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "error/value requires one Error value"), Expr{}
	}
	if arguments[0].kind != exprError {
		return expressionError(span, "expected an Error, got %s", arguments[0].String()), Expr{}
	}
	return nil, arguments[0].error.value
}

func caughtError(err error, span Span) Expr {
	if raised, ok := err.(*raisedError); ok {
		return raised.value
	}
	return errorExpression(stringValue(err.Error(), Span{}), span)
}
