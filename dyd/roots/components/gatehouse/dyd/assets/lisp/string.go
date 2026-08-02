package lisp

import (
	"strings"
	"unicode/utf8"
)

func isString(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "string? requires one argument"), Expr{}
	}
	return nil, boolean(arguments[0].kind == exprString, Span{})
}

func stringLength(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "string/length requires one argument"), Expr{}
	}
	err, value := requireString(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	return nil, integer(int64(utf8.RuneCountInString(value)), Span{})
}

func stringConcat(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	var builder strings.Builder
	for _, argument := range arguments {
		err, value := requireString(argument, span)
		if err != nil {
			return err, Expr{}
		}
		builder.WriteString(value)
	}
	return nil, stringValue(builder.String(), Span{})
}

func stringSlice(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 3 {
		return expressionError(span, "string/slice requires a string, start, and end"), Expr{}
	}
	err, value := requireString(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	err, start := requireInteger(arguments[1], span)
	if err != nil {
		return err, Expr{}
	}
	err, end := requireInteger(arguments[2], span)
	if err != nil {
		return err, Expr{}
	}
	runes := []rune(value)
	if start < 0 || end < start || end > int64(len(runes)) {
		return expressionError(span, "string/slice indices are out of range"), Expr{}
	}
	return nil, stringValue(string(runes[start:end]), Span{})
}

func stringTrim(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "string/trim requires one argument"), Expr{}
	}
	err, value := requireString(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	return nil, stringValue(strings.TrimSpace(value), Span{})
}

func stringLower(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "string/lower requires one argument"), Expr{}
	}
	err, value := requireString(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	return nil, stringValue(strings.ToLower(value), Span{})
}

func stringUpper(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "string/upper requires one argument"), Expr{}
	}
	err, value := requireString(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	return nil, stringValue(strings.ToUpper(value), Span{})
}

func stringContains(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	err, value, part := stringParts(arguments, span, "string/contains?")
	if err != nil {
		return err, Expr{}
	}
	return nil, boolean(strings.Contains(value, part), Span{})
}

func stringPrefix(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	err, value, prefix := stringParts(arguments, span, "string/prefix?")
	if err != nil {
		return err, Expr{}
	}
	return nil, boolean(strings.HasPrefix(value, prefix), Span{})
}

func stringSuffix(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	err, value, suffix := stringParts(arguments, span, "string/suffix?")
	if err != nil {
		return err, Expr{}
	}
	return nil, boolean(strings.HasSuffix(value, suffix), Span{})
}

func stringSplit(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	err, value, separator := stringParts(arguments, span, "string/split")
	if err != nil {
		return err, Expr{}
	}
	if separator == "" {
		runes := []rune(value)
		parts := make([]Expr, len(runes))
		for index, rune := range runes {
			parts[index] = stringValue(string(rune), Span{})
		}
		return nil, list(parts, Span{})
	}
	parts := strings.Split(value, separator)
	values := make([]Expr, len(parts))
	for index, part := range parts {
		values[index] = stringValue(part, Span{})
	}
	return nil, list(values, Span{})
}

func stringJoin(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError(span, "string/join requires a list and separator"), Expr{}
	}
	err, values := expressions(arguments[0])
	if err != nil {
		return err, Expr{}
	}
	err, separator := requireString(arguments[1], span)
	if err != nil {
		return err, Expr{}
	}
	parts := make([]string, len(values))
	for index, value := range values {
		err, part := requireString(value, span)
		if err != nil {
			return err, Expr{}
		}
		parts[index] = part
	}
	return nil, stringValue(strings.Join(parts, separator), Span{})
}

func stringReplace(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 3 {
		return expressionError(span, "string/replace requires text, old text, and new text"), Expr{}
	}
	err, value := requireString(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	err, old := requireString(arguments[1], span)
	if err != nil {
		return err, Expr{}
	}
	err, replacement := requireString(arguments[2], span)
	if err != nil {
		return err, Expr{}
	}
	if old == "" {
		return expressionError(span, "string/replace requires non-empty old text"), Expr{}
	}
	return nil, stringValue(strings.ReplaceAll(value, old, replacement), Span{})
}

func stringParts(arguments []Expr, span Span, name string) (error, string, string) {
	if len(arguments) != 2 {
		return expressionError(span, "%s requires text and a string", name), "", ""
	}
	err, value := requireString(arguments[0], span)
	if err != nil {
		return err, "", ""
	}
	err, part := requireString(arguments[1], span)
	if err != nil {
		return err, "", ""
	}
	return nil, value, part
}

func requireString(expr Expr, span Span) (error, string) {
	if expr.kind != exprString {
		return expressionError(span, "expected a string, got %s", expr.String()), ""
	}
	return nil, expr.text
}
