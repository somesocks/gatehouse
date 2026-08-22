package lisp

import (
	"fmt"
	"time"
)

type Error struct {
	Message string
}

func (err Error) Error() string {
	return err.Message
}

func expressionError(format string, args ...any) error {
	return Error{Message: fmt.Sprintf(format, args...)}
}

type raisedError struct {
	value Expr
}

func (err *raisedError) Error() string {
	if hasSecret(err.value) {
		return "#<secret>"
	}
	base, _ := unwrap(err.value)
	return "thrown error: " + base.(*errorValue).value.String()
}

func errorExpression(value Expr) Expr {
	result := Expr(&errorValue{value: value})
	if hasSecret(value) {
		return withSecret(result)
	}
	return result
}

func catchError(evaluator *evaluator, env *environment, forms []Expr) (error, Expr) {
	if len(forms) != 1 {
		return expressionError("error/catch requires one expression"), nil
	}
	err, result := evaluator.eval(forms[0], env)
	if err == nil {
		return nil, result
	}
	return nil, caughtError(err)
}

func throwError(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("error/throw requires one value"), nil
	}
	value := arguments[0]
	base, _ := unwrap(value)
	if _, ok := base.(*errorValue); !ok {
		value = errorExpression(value)
	}
	return &raisedError{value: value}, nil
}

func assertValue(evaluator *evaluator, env *environment, forms []Expr) (error, Expr) {
	if len(forms) < 1 || len(forms) > 3 {
		return expressionError("assert requires a condition and optional value and failure"), nil
	}
	err, condition := evaluator.eval(forms[0], env)
	if err != nil {
		return err, nil
	}
	err, valid := requireBoolean(condition)
	if err != nil {
		return err, nil
	}
	if !valid {
		if len(forms) == 3 {
			err, value := evaluator.eval(forms[2], env)
			if err != nil {
				return err, nil
			}
			base, _ := unwrap(value)
			if _, ok := base.(*errorValue); !ok {
				value = errorExpression(value)
			}
			return &raisedError{value: value}, nil
		}
		return &raisedError{value: errorExpression(stringValue("assertion failed"))}, nil
	}
	if len(forms) == 1 {
		return nil, null()
	}
	return evaluator.eval(forms[1], env)
}

func assertTiming(evaluator *evaluator, env *environment, forms []Expr) (error, Expr) {
	if len(forms) != 2 {
		return expressionError("assert/timing requires maximum milliseconds and one expression"), nil
	}
	err, maximum := evaluator.eval(forms[0], env)
	if err != nil {
		return err, nil
	}
	err, milliseconds := requireInteger(maximum)
	if err != nil {
		return err, nil
	}
	if milliseconds < 0 {
		return expressionError("assert/timing requires non-negative maximum milliseconds"), nil
	}

	started := time.Now()
	err, result := evaluator.eval(forms[1], env)
	if err != nil {
		return err, nil
	}
	elapsed := time.Since(started).Milliseconds()
	if elapsed > milliseconds {
		return expressionError("assert/timing exceeded %d milliseconds (actual %d milliseconds)", milliseconds, elapsed), nil
	}
	return nil, result
}

func isError(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("error? requires one argument"), nil
	}
	base, _ := unwrap(arguments[0])
	_, ok := base.(*errorValue)
	return nil, boolean(ok)
}

func errorValueOf(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("error/value requires one Error value"), nil
	}
	base, _ := unwrap(arguments[0])
	value, ok := base.(*errorValue)
	if !ok {
		return expressionError("expected an Error, got %s", arguments[0].String()), nil
	}
	return nil, value.value
}

func caughtError(err error) Expr {
	if raised, ok := err.(*raisedError); ok {
		return raised.value
	}
	return errorExpression(stringValue(err.Error()))
}
