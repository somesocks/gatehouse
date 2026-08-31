package lisp

import (
	"errors"
	"fmt"
	"time"
)

// ErrInterrupted reports that an evaluation context was cancelled or expired.
var ErrInterrupted = errors.New("evaluation interrupted")

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

type taintedError struct {
	cause error
	taint Taint
}

type interruptedError struct{ cause error }

func (err interruptedError) Error() string { return ErrInterrupted.Error() }
func (err interruptedError) Unwrap() error { return err.cause }
func (err interruptedError) Is(target error) bool { return target == ErrInterrupted }

func (err *raisedError) Error() string {
	if TaintOf(err.value) != TaintNone {
		return err.value.String()
	}
	base, _ := unwrap(err.value)
	return "thrown error: " + base.(*errorValue).value.String()
}

func (err *taintedError) Error() string {
	return withTaint(null(), err.taint).String()
}

func (err *taintedError) Unwrap() error { return err.cause }

func taintError(err error, taint Taint) error {
	if taint == TaintNone {
		return err
	}
	if existing, ok := err.(*taintedError); ok {
		existing.taint = joinTaint(existing.taint, taint)
		return existing
	}
	return &taintedError{cause: err, taint: taint}
}

func errorExpression(value Expr) Expr {
	result := Expr(&errorValue{value: value})
	if taint := TaintOf(value); taint != TaintNone {
		return withTaint(result, taint)
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
	if errors.Is(err, ErrInterrupted) {
		return err, nil
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

func assertValues(evaluator *evaluator, env *environment, forms []Expr) (error, callOutcome) {
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
			return &raisedError{value: withTaint(value, TaintOf(condition))}, nil
		}
		return &raisedError{value: withTaint(errorExpression(stringValue("assertion failed")), TaintOf(condition))}, nil
	}
	if len(forms) == 1 {
		return nil, callResult{value: withTaint(null(), TaintOf(condition))}
	}
	return nil, callTailState{expression: forms[1], taint: TaintOf(condition)}
}

func assertTimingValues(evaluator *evaluator, env *environment, forms []Expr) (error, callOutcome) {
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
		return taintError(expressionError("assert/timing requires non-negative maximum milliseconds"), TaintOf(maximum)), nil
	}

	started := time.Now()
	err, result := evaluator.eval(forms[1], env)
	if err != nil {
		return err, nil
	}
	elapsed := time.Since(started).Milliseconds()
	if elapsed > milliseconds {
		return taintError(expressionError("assert/timing exceeded %d milliseconds (actual %d milliseconds)", milliseconds, elapsed), TaintOf(maximum)), nil
	}
	return nil, callResult{value: withTaint(result, TaintOf(maximum))}
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
	if tainted, ok := err.(*taintedError); ok {
		return errorExpression(withTaint(stringValue(tainted.cause.Error()), tainted.taint))
	}
	return errorExpression(stringValue(err.Error()))
}
