package lisp

import (
	"strings"
	"unicode/utf8"
)

func isString(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("string? requires one argument"), nil
	}
	base, _ := unwrap(arguments[0])
	_, ok := base.(*stringExpr)
	return nil, boolean(ok)
}
func stringLength(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("string/length requires one argument"), nil
	}
	err, value := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, integer(int64(utf8.RuneCountInString(value)))
}
func stringConcat(_ *evaluator, arguments []Expr) (error, Expr) {
	var builder strings.Builder
	for _, argument := range arguments {
		err, value := requireString(argument)
		if err != nil {
			return err, nil
		}
		builder.WriteString(value)
	}
	return nil, stringValue(builder.String())
}
func stringSlice(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 3 {
		return expressionError("string/slice requires a string, start, and end"), nil
	}
	err, value := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	err, start := requireInteger(arguments[1])
	if err != nil {
		return err, nil
	}
	err, end := requireInteger(arguments[2])
	if err != nil {
		return err, nil
	}
	runes := []rune(value)
	if start < 0 || end < start || end > int64(len(runes)) {
		return expressionError("string/slice indices are out of range"), nil
	}
	return nil, stringValue(string(runes[start:end]))
}
func stringTrim(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("string/trim requires one argument"), nil
	}
	err, value := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, stringValue(strings.TrimSpace(value))
}
func stringLower(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("string/lower requires one argument"), nil
	}
	err, value := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, stringValue(strings.ToLower(value))
}
func stringUpper(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("string/upper requires one argument"), nil
	}
	err, value := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, stringValue(strings.ToUpper(value))
}
func stringContains(_ *evaluator, arguments []Expr) (error, Expr) {
	err, value, part := stringParts(arguments, "string/contains?")
	if err != nil {
		return err, nil
	}
	return nil, boolean(strings.Contains(value, part))
}
func stringPrefix(_ *evaluator, arguments []Expr) (error, Expr) {
	err, value, prefix := stringParts(arguments, "string/prefix?")
	if err != nil {
		return err, nil
	}
	return nil, boolean(strings.HasPrefix(value, prefix))
}
func stringSuffix(_ *evaluator, arguments []Expr) (error, Expr) {
	err, value, suffix := stringParts(arguments, "string/suffix?")
	if err != nil {
		return err, nil
	}
	return nil, boolean(strings.HasSuffix(value, suffix))
}
func stringSplit(_ *evaluator, arguments []Expr) (error, Expr) {
	err, value, separator := stringParts(arguments, "string/split")
	if err != nil {
		return err, nil
	}
	if separator == "" {
		runes := []rune(value)
		parts := make([]Expr, len(runes))
		for index, rune := range runes {
			parts[index] = stringValue(string(rune))
		}
		return nil, list(parts)
	}
	parts := strings.Split(value, separator)
	values := make([]Expr, len(parts))
	for index, part := range parts {
		values[index] = stringValue(part)
	}
	return nil, list(values)
}
func stringJoin(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError("string/join requires a list and separator"), nil
	}
	err, values := expressions(arguments[0])
	if err != nil {
		return err, nil
	}
	err, separator := requireString(arguments[1])
	if err != nil {
		return err, nil
	}
	parts := make([]string, len(values))
	for index, value := range values {
		err, part := requireString(value)
		if err != nil {
			return err, nil
		}
		parts[index] = part
	}
	return nil, stringValue(strings.Join(parts, separator))
}
func stringReplace(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 3 {
		return expressionError("string/replace requires text, old text, and new text"), nil
	}
	err, value := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	err, old := requireString(arguments[1])
	if err != nil {
		return err, nil
	}
	err, replacement := requireString(arguments[2])
	if err != nil {
		return err, nil
	}
	if old == "" {
		return expressionError("string/replace requires non-empty old text"), nil
	}
	return nil, stringValue(strings.ReplaceAll(value, old, replacement))
}
func stringParts(arguments []Expr, name string) (error, string, string) {
	if len(arguments) != 2 {
		return expressionError("%s requires text and a string", name), "", ""
	}
	err, value := requireString(arguments[0])
	if err != nil {
		return err, "", ""
	}
	err, part := requireString(arguments[1])
	if err != nil {
		return err, "", ""
	}
	return nil, value, part
}
func requireString(expr Expr) (error, string) {
	base, _ := unwrap(expr)
	value, ok := base.(*stringExpr)
	if !ok {
		return expressionError("expected a string, got %s", expr.String()), ""
	}
	return nil, value.value
}
