package lisp

import "math"

func prelude() *environment {
	env := &environment{values: make(map[string]*Expr)}
	env.bind("null", null(Span{}))
	for name, call := range map[string]func(*evaluator, []Expr, Span) (error, Expr){
		"+":                add,
		"-":                subtract,
		"*":                multiply,
		"bool?":            isBoolean,
		"int?":             isInteger,
		"symbol?":          isSymbolValue,
		"int/div":          integerDivide,
		"int/rem":          integerRemainder,
		"int/abs":          integerAbsolute,
		"int/min":          integerMinimum,
		"int/max":          integerMaximum,
		"=":                equals,
		"<":                lessThan,
		"<=":               lessThanOrEqual,
		">":                greaterThan,
		">=":               greaterThanOrEqual,
		"not":              logicalNot,
		"pair":             makePair,
		"head":             head,
		"tail":             tail,
		"list":             makeList,
		"pair?":            isPair,
		"null?":            isNull,
		"list?":            isList,
		"list/length":      listLength,
		"list/concat":      concatLists,
		"list/map":         mapValues,
		"list/filter":      filterValues,
		"list/fold":        foldValues,
		"string?":          isString,
		"string/length":    stringLength,
		"string/concat":    stringConcat,
		"string/slice":     stringSlice,
		"string/trim":      stringTrim,
		"string/lower":     stringLower,
		"string/upper":     stringUpper,
		"string/contains?": stringContains,
		"string/prefix?":   stringPrefix,
		"string/suffix?":   stringSuffix,
		"string/split":     stringSplit,
		"string/join":      stringJoin,
		"string/replace":   stringReplace,
	} {
		env.bind(name, Expr{kind: exprBuiltin, builtin: &builtin{name: name, call: call}})
	}
	return env
}

func (env *environment) bind(name string, value Expr) {
	bound := value
	env.values[name] = &bound
}

func add(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	var total int64
	for _, argument := range arguments {
		err, value := requireInteger(argument, span)
		if err != nil {
			return err, Expr{}
		}
		if (value > 0 && total > math.MaxInt64-value) || (value < 0 && total < math.MinInt64-value) {
			return expressionError(span, "integer overflow"), Expr{}
		}
		total += value
	}
	return nil, integer(total, Span{})
}

func subtract(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) == 0 {
		return expressionError(span, "- requires at least one argument"), Expr{}
	}
	err, total := requireInteger(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	if len(arguments) == 1 {
		if total == math.MinInt64 {
			return expressionError(span, "integer overflow"), Expr{}
		}
		return nil, integer(-total, Span{})
	}
	for _, argument := range arguments[1:] {
		err, value := requireInteger(argument, span)
		if err != nil {
			return err, Expr{}
		}
		if (value > 0 && total < math.MinInt64+value) || (value < 0 && total > math.MaxInt64+value) {
			return expressionError(span, "integer overflow"), Expr{}
		}
		total -= value
	}
	return nil, integer(total, Span{})
}

func multiply(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	total := int64(1)
	for _, argument := range arguments {
		err, value := requireInteger(argument, span)
		if err != nil {
			return err, Expr{}
		}
		if multiplicationOverflows(total, value) {
			return expressionError(span, "integer overflow"), Expr{}
		}
		total *= value
	}
	return nil, integer(total, Span{})
}

func isBoolean(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "bool? requires one argument"), Expr{}
	}
	return nil, boolean(arguments[0].kind == exprBoolean, Span{})
}

func isInteger(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "int? requires one argument"), Expr{}
	}
	return nil, boolean(arguments[0].kind == exprInteger, Span{})
}

func isSymbolValue(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "symbol? requires one argument"), Expr{}
	}
	return nil, boolean(arguments[0].kind == exprSymbol, Span{})
}

func integerDivide(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	err, dividend, divisor := integerOperands(arguments, span, "int/div")
	if err != nil {
		return err, Expr{}
	}
	if divisor == 0 {
		return expressionError(span, "int/div requires a non-zero divisor"), Expr{}
	}
	if dividend == math.MinInt64 && divisor == -1 {
		return expressionError(span, "integer overflow"), Expr{}
	}
	return nil, integer(dividend/divisor, Span{})
}

func integerRemainder(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	err, dividend, divisor := integerOperands(arguments, span, "int/rem")
	if err != nil {
		return err, Expr{}
	}
	if divisor == 0 {
		return expressionError(span, "int/rem requires a non-zero divisor"), Expr{}
	}
	return nil, integer(dividend%divisor, Span{})
}

func integerAbsolute(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "int/abs requires one integer"), Expr{}
	}
	err, value := requireInteger(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	if value == math.MinInt64 {
		return expressionError(span, "integer overflow"), Expr{}
	}
	if value < 0 {
		value = -value
	}
	return nil, integer(value, Span{})
}

func integerMinimum(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	return integerExtremum(arguments, span, "int/min", func(candidate int64, current int64) bool {
		return candidate < current
	})
}

func integerMaximum(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	return integerExtremum(arguments, span, "int/max", func(candidate int64, current int64) bool {
		return candidate > current
	})
}

func equals(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) < 2 {
		return expressionError(span, "= requires at least two arguments"), Expr{}
	}
	for index := 1; index < len(arguments); index++ {
		if !equal(arguments[index-1], arguments[index]) {
			return nil, boolean(false, Span{})
		}
	}
	return nil, boolean(true, Span{})
}

func lessThan(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	return compareIntegers(arguments, span, func(left int64, right int64) bool { return left < right }, "<")
}

func lessThanOrEqual(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	return compareIntegers(arguments, span, func(left int64, right int64) bool { return left <= right }, "<=")
}

func greaterThan(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	return compareIntegers(arguments, span, func(left int64, right int64) bool { return left > right }, ">")
}

func greaterThanOrEqual(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	return compareIntegers(arguments, span, func(left int64, right int64) bool { return left >= right }, ">=")
}

func compareIntegers(arguments []Expr, span Span, compare func(int64, int64) bool, name string) (error, Expr) {
	if len(arguments) < 2 {
		return expressionError(span, "%s requires at least two arguments", name), Expr{}
	}
	err, previous := requireInteger(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	for _, argument := range arguments[1:] {
		err, current := requireInteger(argument, span)
		if err != nil {
			return err, Expr{}
		}
		if !compare(previous, current) {
			return nil, boolean(false, Span{})
		}
		previous = current
	}
	return nil, boolean(true, Span{})
}

func logicalNot(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "not requires one argument"), Expr{}
	}
	err, value := requireBoolean(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	return nil, boolean(!value, Span{})
}

func makePair(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError(span, "pair requires two arguments"), Expr{}
	}
	return nil, pairValue(arguments[0], arguments[1], Span{})
}

func head(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "head requires one argument"), Expr{}
	}
	if arguments[0].kind != exprPair {
		return expressionError(span, "head requires a pair"), Expr{}
	}
	return nil, arguments[0].pair.first
}

func tail(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "tail requires one argument"), Expr{}
	}
	if arguments[0].kind != exprPair {
		return expressionError(span, "tail requires a pair"), Expr{}
	}
	return nil, arguments[0].pair.rest
}

func makeList(_ *evaluator, arguments []Expr, _ Span) (error, Expr) {
	return nil, list(arguments, Span{})
}

func isPair(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "pair? requires one argument"), Expr{}
	}
	return nil, boolean(arguments[0].kind == exprPair, Span{})
}

func isNull(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "null? requires one argument"), Expr{}
	}
	return nil, boolean(arguments[0].kind == exprNull, Span{})
}

func isList(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "list? requires one argument"), Expr{}
	}
	return nil, boolean(isProperList(arguments[0]), Span{})
}

func listLength(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "list/length requires one argument"), Expr{}
	}
	err, values := expressions(arguments[0])
	if err != nil {
		return err, Expr{}
	}
	return nil, integer(int64(len(values)), Span{})
}

func concatLists(_ *evaluator, arguments []Expr, _ Span) (error, Expr) {
	var result []Expr
	for _, argument := range arguments {
		err, values := expressions(argument)
		if err != nil {
			return err, Expr{}
		}
		result = append(result, values...)
	}
	return nil, list(result, Span{})
}

func mapValues(evaluator *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError(span, "list/map requires a function and list"), Expr{}
	}
	err, values := expressions(arguments[1])
	if err != nil {
		return err, Expr{}
	}
	result := make([]Expr, 0, len(values))
	for _, value := range values {
		err, mapped := evaluator.call(arguments[0], []Expr{value}, span)
		if err != nil {
			return err, Expr{}
		}
		result = append(result, mapped)
	}
	return nil, list(result, Span{})
}

func filterValues(evaluator *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError(span, "list/filter requires a function and list"), Expr{}
	}
	err, values := expressions(arguments[1])
	if err != nil {
		return err, Expr{}
	}
	result := make([]Expr, 0, len(values))
	for _, value := range values {
		err, keep := evaluator.call(arguments[0], []Expr{value}, span)
		if err != nil {
			return err, Expr{}
		}
		err, include := requireBoolean(keep, span)
		if err != nil {
			return err, Expr{}
		}
		if include {
			result = append(result, value)
		}
	}
	return nil, list(result, Span{})
}

func foldValues(evaluator *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 3 {
		return expressionError(span, "list/fold requires a function, initial value, and list"), Expr{}
	}
	err, values := expressions(arguments[2])
	if err != nil {
		return err, Expr{}
	}
	result := arguments[1]
	for _, value := range values {
		err, result = evaluator.call(arguments[0], []Expr{result, value}, span)
		if err != nil {
			return err, Expr{}
		}
	}
	return nil, result
}

func requireInteger(expr Expr, span Span) (error, int64) {
	if expr.kind != exprInteger {
		return expressionError(span, "expected an integer, got %s", expr.String()), 0
	}
	return nil, expr.integer
}

func integerOperands(arguments []Expr, span Span, name string) (error, int64, int64) {
	if len(arguments) != 2 {
		return expressionError(span, "%s requires two integers", name), 0, 0
	}
	err, dividend := requireInteger(arguments[0], span)
	if err != nil {
		return err, 0, 0
	}
	err, divisor := requireInteger(arguments[1], span)
	if err != nil {
		return err, 0, 0
	}
	return nil, dividend, divisor
}

func integerExtremum(arguments []Expr, span Span, name string, prefer func(candidate int64, current int64) bool) (error, Expr) {
	if len(arguments) == 0 {
		return expressionError(span, "%s requires at least one integer", name), Expr{}
	}
	err, result := requireInteger(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	for _, argument := range arguments[1:] {
		err, value := requireInteger(argument, span)
		if err != nil {
			return err, Expr{}
		}
		if prefer(value, result) {
			result = value
		}
	}
	return nil, integer(result, Span{})
}

func isProperList(expr Expr) bool {
	for expr.kind == exprPair {
		expr = expr.pair.rest
	}
	return expr.kind == exprNull
}

func multiplicationOverflows(left int64, right int64) bool {
	if left == 0 || right == 0 {
		return false
	}
	if left == -1 {
		return right == math.MinInt64
	}
	if right == -1 {
		return left == math.MinInt64
	}
	return left*right/right != left
}
