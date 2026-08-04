package lisp

import "math"

func prelude() *environment {
	env := &environment{values: make(map[string]*Expr)}
	env.bind("null", null(Span{}))
	for _, definition := range preludeBuiltins {
		env.bind(definition.name, Expr{
			kind: exprBuiltin,
			help: definition.help,
			builtin: &builtin{
				special: definition.special,
				leaky:   definition.leaky,
				call:    definition.call,
			},
		})
	}
	env.bind("import", importBuiltin(&moduleLoader{cache: newModuleCache()}))
	return env
}

type builtinDefinition struct {
	name    string
	help    string
	special bool
	leaky   bool
	call    builtinCall
}

var preludeBuiltins = []builtinDefinition{
	{name: "+", help: "Adds integer values.", leaky: true, call: pure(add)},
	{name: "-", help: "Subtracts integer values or negates one integer.", leaky: true, call: pure(subtract)},
	{name: "*", help: "Multiplies integer values.", leaky: true, call: pure(multiply)},
	{name: "bool?", help: "Returns whether a value is a Boolean.", call: pure(isBoolean)},
	{name: "int?", help: "Returns whether a value is an integer.", call: pure(isInteger)},
	{name: "symbol?", help: "Returns whether a value is a Symbol.", call: pure(isSymbolValue)},
	{name: "int/div", help: "Divides integers with truncation toward zero.", leaky: true, call: pure(integerDivide)},
	{name: "int/rem", help: "Returns the signed remainder of integer division.", leaky: true, call: pure(integerRemainder)},
	{name: "int/abs", help: "Returns the non-negative magnitude of an integer.", leaky: true, call: pure(integerAbsolute)},
	{name: "int/min", help: "Returns the smallest supplied integer.", leaky: true, call: pure(integerMinimum)},
	{name: "int/max", help: "Returns the largest supplied integer.", leaky: true, call: pure(integerMaximum)},
	{name: "=", help: "Returns whether values have equal types and structural contents.", leaky: true, call: pure(equals)},
	{name: "<", help: "Returns whether integers are strictly increasing.", leaky: true, call: pure(lessThan)},
	{name: "<=", help: "Returns whether integers are nondecreasing.", leaky: true, call: pure(lessThanOrEqual)},
	{name: ">", help: "Returns whether integers are strictly decreasing.", leaky: true, call: pure(greaterThan)},
	{name: ">=", help: "Returns whether integers are nonincreasing.", leaky: true, call: pure(greaterThanOrEqual)},
	{name: "not", help: "Negates a Boolean value.", leaky: true, call: pure(logicalNot)},
	{name: "pair", help: "Creates a pair from two values.", leaky: true, call: pure(makePair)},
	{name: "head", help: "Returns the first value of a pair.", leaky: true, call: pure(head)},
	{name: "tail", help: "Returns the second value of a pair.", leaky: true, call: pure(tail)},
	{name: "list", help: "Creates a proper list from its arguments.", leaky: true, call: pure(makeList)},
	{name: "pair?", help: "Returns whether a value is a pair.", call: pure(isPair)},
	{name: "null?", help: "Returns whether a value is the empty list.", call: pure(isNull)},
	{name: "list?", help: "Returns whether a value is a proper list.", call: pure(isList)},
	{name: "list/length", help: "Returns the number of values in a proper list.", call: pure(listLength)},
	{name: "list/concat", help: "Concatenates proper lists.", leaky: true, call: pure(concatLists)},
	{name: "list/map", help: "Applies a function to every value in a proper list.", leaky: true, call: mapValues},
	{name: "list/filter", help: "Keeps list values whose predicate returns true.", leaky: true, call: filterValues},
	{name: "list/fold", help: "Combines a proper list from left to right with an accumulator.", leaky: true, call: foldValues},
	{name: "string?", help: "Returns whether a value is a string.", call: pure(isString)},
	{name: "string/length", help: "Returns the Unicode code-point length of text.", call: pure(stringLength)},
	{name: "bytes?", help: "Returns whether a value is Bytes.", call: pure(isBytes)},
	{name: "bytes/length", help: "Returns the number of bytes in a Bytes value.", call: pure(bytesLength)},
	{name: "bytes/concat", help: "Concatenates Bytes values.", leaky: true, call: pure(bytesConcat)},
	{name: "bytes/slice", help: "Returns a byte slice between an inclusive start and exclusive end.", leaky: true, call: pure(bytesSlice)},
	{name: "bytes/utf8/encode", help: "Encodes a string as UTF-8 Bytes.", leaky: true, call: pure(bytesUTF8Encode)},
	{name: "bytes/utf8/decode", help: "Decodes valid UTF-8 Bytes as a string.", leaky: true, call: pure(bytesUTF8Decode)},
	{name: "bytes/hex/encode", help: "Encodes Bytes as lower-case hexadecimal text.", leaky: true, call: pure(bytesHexEncode)},
	{name: "bytes/hex/decode", help: "Decodes hexadecimal text as Bytes.", leaky: true, call: pure(bytesHexDecode)},
	{name: "bytes/base64/encode", help: "Encodes Bytes as standard padded Base64 text.", leaky: true, call: pure(bytesBase64Encode)},
	{name: "bytes/base64/decode", help: "Decodes standard padded Base64 text as Bytes.", leaky: true, call: pure(bytesBase64Decode)},
	{name: "bytes/base64url/encode", help: "Encodes Bytes as URL-safe padded Base64 text.", leaky: true, call: pure(bytesBase64URLEncode)},
	{name: "bytes/base64url/decode", help: "Decodes URL-safe padded Base64 text as Bytes.", leaky: true, call: pure(bytesBase64URLDecode)},
	{name: "bytes/base64/pad", help: "Normalizes standard or URL-safe Base64 text with padding.", leaky: true, call: pure(bytesBase64Pad)},
	{name: "bytes/base64/unpad", help: "Normalizes standard or URL-safe Base64 text without padding.", leaky: true, call: pure(bytesBase64Unpad)},
	{name: "string/concat", help: "Concatenates string values.", leaky: true, call: pure(stringConcat)},
	{name: "string/slice", help: "Returns a Unicode code-point substring slice between an inclusive start and exclusive end.", leaky: true, call: pure(stringSlice)},
	{name: "string/trim", help: "Removes Unicode whitespace from both ends of text.", leaky: true, call: pure(stringTrim)},
	{name: "string/lower", help: "Converts text to locale-independent Unicode lower case.", leaky: true, call: pure(stringLower)},
	{name: "string/upper", help: "Converts text to locale-independent Unicode upper case.", leaky: true, call: pure(stringUpper)},
	{name: "string/contains?", help: "Returns whether text contains a string.", call: pure(stringContains)},
	{name: "string/prefix?", help: "Returns whether text starts with a string.", call: pure(stringPrefix)},
	{name: "string/suffix?", help: "Returns whether text ends with a string.", call: pure(stringSuffix)},
	{name: "string/split", help: "Splits text on a literal separator into a list of strings.", leaky: true, call: pure(stringSplit)},
	{name: "string/join", help: "Joins a list of strings with a separator.", leaky: true, call: pure(stringJoin)},
	{name: "string/replace", help: "Replaces every non-overlapping literal occurrence in text.", leaky: true, call: pure(stringReplace)},
	{name: "error/throw", help: "Raises an Error with a value.", call: pure(throwError)},
	{name: "error/catch", help: "Returns an Error raised while evaluating an expression.", special: true, call: catchError},
	{name: "error?", help: "Returns whether a value is an Error.", call: pure(isError)},
	{name: "error/value", help: "Returns the value carried by an Error.", leaky: true, call: pure(errorValueOf)},
	{name: "assert", help: "Returns null or a supplied value when a condition is true, otherwise raises an Error.", special: true, call: assertValue},
	{name: "secret?", help: "Returns whether a value is secret-tainted.", call: pure(isSecret)},
	{name: "secret/mark", help: "Marks any value as secret-tainted.", call: pure(markSecret)},
	{name: "help", help: "Returns help text for a value.", call: helpValue},
	{name: "help/document", help: "Returns a value with public help text attached.", call: pure(helpDocument)},
	{name: "help/env", help: "Returns visible lexical binding names matching every search term.", call: helpEnv},
	{name: "help/search", help: "Returns visible documented bindings whose help text matches every search term.", call: helpSearch},
	{name: "import/restrict", help: "Evaluates a body with an allowlisted importer.", special: true, call: importRestrict},
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

func isSecret(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "secret? requires one argument"), Expr{}
	}
	return nil, boolean(arguments[0].secret, Span{})
}

func markSecret(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "secret/mark requires one argument"), Expr{}
	}
	result := arguments[0]
	result.secret = true
	return nil, result
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
