package lisp

import "math"

func prelude() *environment {
	env := &environment{values: make(map[string]*Expr)}
	env.bind("null", null(Span{}))
	for _, definition := range preludeBuiltins {
		env.bind(definition.name, Expr{kind: exprBuiltin, builtin: &builtin{
			helpText:     definition.helpText,
			helpKeywords: definition.helpKeywords,
			call:         definition.call,
		}})
	}
	return env
}

type builtinDefinition struct {
	name         string
	helpText     string
	helpKeywords []string
	call         builtinCall
}

var preludeBuiltins = []builtinDefinition{
	{name: "+", helpText: "Adds integer values.", helpKeywords: []string{"add", "sum", "integer"}, call: pure(add)},
	{name: "-", helpText: "Subtracts integer values or negates one integer.", helpKeywords: []string{"subtract", "negate", "integer"}, call: pure(subtract)},
	{name: "*", helpText: "Multiplies integer values.", helpKeywords: []string{"multiply", "product", "integer"}, call: pure(multiply)},
	{name: "bool?", helpText: "Returns whether a value is a Boolean.", helpKeywords: []string{"boolean", "type", "predicate"}, call: pure(isBoolean)},
	{name: "int?", helpText: "Returns whether a value is an integer.", helpKeywords: []string{"integer", "number", "type", "predicate"}, call: pure(isInteger)},
	{name: "symbol?", helpText: "Returns whether a value is a Symbol.", helpKeywords: []string{"symbol", "type", "predicate"}, call: pure(isSymbolValue)},
	{name: "int/div", helpText: "Divides integers with truncation toward zero.", helpKeywords: []string{"integer", "division", "quotient"}, call: pure(integerDivide)},
	{name: "int/rem", helpText: "Returns the signed remainder of integer division.", helpKeywords: []string{"integer", "division", "remainder"}, call: pure(integerRemainder)},
	{name: "int/abs", helpText: "Returns the non-negative magnitude of an integer.", helpKeywords: []string{"integer", "absolute", "magnitude"}, call: pure(integerAbsolute)},
	{name: "int/min", helpText: "Returns the smallest supplied integer.", helpKeywords: []string{"integer", "minimum", "smallest"}, call: pure(integerMinimum)},
	{name: "int/max", helpText: "Returns the largest supplied integer.", helpKeywords: []string{"integer", "maximum", "largest"}, call: pure(integerMaximum)},
	{name: "=", helpText: "Returns whether values have equal types and structural contents.", helpKeywords: []string{"equal", "equality", "compare"}, call: pure(equals)},
	{name: "<", helpText: "Returns whether integers are strictly increasing.", helpKeywords: []string{"integer", "compare", "less"}, call: pure(lessThan)},
	{name: "<=", helpText: "Returns whether integers are nondecreasing.", helpKeywords: []string{"integer", "compare", "less", "equal"}, call: pure(lessThanOrEqual)},
	{name: ">", helpText: "Returns whether integers are strictly decreasing.", helpKeywords: []string{"integer", "compare", "greater"}, call: pure(greaterThan)},
	{name: ">=", helpText: "Returns whether integers are nonincreasing.", helpKeywords: []string{"integer", "compare", "greater", "equal"}, call: pure(greaterThanOrEqual)},
	{name: "not", helpText: "Negates a Boolean value.", helpKeywords: []string{"boolean", "negation", "logical"}, call: pure(logicalNot)},
	{name: "pair", helpText: "Creates a pair from two values.", helpKeywords: []string{"pair", "data", "construct"}, call: pure(makePair)},
	{name: "head", helpText: "Returns the first value of a pair.", helpKeywords: []string{"pair", "first", "list"}, call: pure(head)},
	{name: "tail", helpText: "Returns the second value of a pair.", helpKeywords: []string{"pair", "rest", "list"}, call: pure(tail)},
	{name: "list", helpText: "Creates a proper list from its arguments.", helpKeywords: []string{"list", "construct", "collection"}, call: pure(makeList)},
	{name: "pair?", helpText: "Returns whether a value is a pair.", helpKeywords: []string{"pair", "type", "predicate"}, call: pure(isPair)},
	{name: "null?", helpText: "Returns whether a value is the empty list.", helpKeywords: []string{"null", "empty", "list", "predicate"}, call: pure(isNull)},
	{name: "list?", helpText: "Returns whether a value is a proper list.", helpKeywords: []string{"list", "proper", "type", "predicate"}, call: pure(isList)},
	{name: "list/length", helpText: "Returns the number of values in a proper list.", helpKeywords: []string{"list", "length", "count"}, call: pure(listLength)},
	{name: "list/concat", helpText: "Concatenates proper lists.", helpKeywords: []string{"list", "concat", "append"}, call: pure(concatLists)},
	{name: "list/map", helpText: "Applies a function to every value in a proper list.", helpKeywords: []string{"list", "map", "transform"}, call: mapValues},
	{name: "list/filter", helpText: "Keeps list values whose predicate returns true.", helpKeywords: []string{"list", "filter", "select", "predicate"}, call: filterValues},
	{name: "list/fold", helpText: "Combines a proper list from left to right with an accumulator.", helpKeywords: []string{"list", "fold", "reduce", "accumulator"}, call: foldValues},
	{name: "string?", helpText: "Returns whether a value is a string.", helpKeywords: []string{"string", "text", "type", "predicate"}, call: pure(isString)},
	{name: "string/length", helpText: "Returns the Unicode code-point length of text.", helpKeywords: []string{"string", "text", "length", "unicode"}, call: pure(stringLength)},
	{name: "string/concat", helpText: "Concatenates string values.", helpKeywords: []string{"string", "text", "concat", "join"}, call: pure(stringConcat)},
	{name: "string/slice", helpText: "Returns the code-point slice between an inclusive start and exclusive end.", helpKeywords: []string{"string", "text", "substring", "slice", "unicode"}, call: pure(stringSlice)},
	{name: "string/trim", helpText: "Removes Unicode whitespace from both ends of text.", helpKeywords: []string{"string", "text", "trim", "whitespace"}, call: pure(stringTrim)},
	{name: "string/lower", helpText: "Converts text to locale-independent Unicode lower case.", helpKeywords: []string{"string", "text", "lowercase", "case"}, call: pure(stringLower)},
	{name: "string/upper", helpText: "Converts text to locale-independent Unicode upper case.", helpKeywords: []string{"string", "text", "uppercase", "case"}, call: pure(stringUpper)},
	{name: "string/contains?", helpText: "Returns whether text contains a string.", helpKeywords: []string{"string", "text", "contains", "search"}, call: pure(stringContains)},
	{name: "string/prefix?", helpText: "Returns whether text starts with a string.", helpKeywords: []string{"string", "text", "prefix", "starts"}, call: pure(stringPrefix)},
	{name: "string/suffix?", helpText: "Returns whether text ends with a string.", helpKeywords: []string{"string", "text", "suffix", "ends"}, call: pure(stringSuffix)},
	{name: "string/split", helpText: "Splits text on a literal separator into a list of strings.", helpKeywords: []string{"string", "text", "split", "separator"}, call: pure(stringSplit)},
	{name: "string/join", helpText: "Joins a list of strings with a separator.", helpKeywords: []string{"string", "text", "join", "separator"}, call: pure(stringJoin)},
	{name: "string/replace", helpText: "Replaces every non-overlapping literal occurrence in text.", helpKeywords: []string{"string", "text", "replace", "substitute"}, call: pure(stringReplace)},
	{name: "help", helpText: "Returns documentation for a value.", helpKeywords: []string{"help", "documentation", "describe", "type"}, call: helpValue},
	{name: "help/search", helpText: "Returns visible documented bindings matching every search term.", helpKeywords: []string{"help", "search", "discover", "documentation", "function"}, call: helpSearch},
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

func mapValues(evaluator *evaluator, env *environment, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError(span, "list/map requires a function and list"), Expr{}
	}
	err, values := expressions(arguments[1])
	if err != nil {
		return err, Expr{}
	}
	result := make([]Expr, 0, len(values))
	for _, value := range values {
		err, mapped := evaluator.call(arguments[0], env, []Expr{value}, span)
		if err != nil {
			return err, Expr{}
		}
		result = append(result, mapped)
	}
	return nil, list(result, Span{})
}

func filterValues(evaluator *evaluator, env *environment, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError(span, "list/filter requires a function and list"), Expr{}
	}
	err, values := expressions(arguments[1])
	if err != nil {
		return err, Expr{}
	}
	result := make([]Expr, 0, len(values))
	for _, value := range values {
		err, keep := evaluator.call(arguments[0], env, []Expr{value}, span)
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

func foldValues(evaluator *evaluator, env *environment, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 3 {
		return expressionError(span, "list/fold requires a function, initial value, and list"), Expr{}
	}
	err, values := expressions(arguments[2])
	if err != nil {
		return err, Expr{}
	}
	result := arguments[1]
	for _, value := range values {
		err, result = evaluator.call(arguments[0], env, []Expr{result, value}, span)
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
