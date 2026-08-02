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
