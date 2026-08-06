package lisp

import "math"

func prelude() *environment {
	env := bootstrap()
	bindImports(env, newModuleCache())
	return env
}

func bootstrap() *environment {
	env := &environment{values: make(map[string]*Expr)}
	env.bind("null", null())
	for _, definition := range preludeBuiltins {
		env.bind(definition.name, withHelp(&builtin{special: definition.special, leaky: definition.leaky, call: definition.call}, definition.documentation.text()))
	}
	return env
}

type builtinDefinition struct {
	name          string
	documentation documentation
	special       bool
	leaky         bool
	call          builtinCall
}

var preludeBuiltins = []builtinDefinition{
	{name: "+", documentation: doc("(+ integer...) -> Integer", "Adds zero or more integers.", "(+ 1 2 3)", "6"), leaky: true, call: pure(add)},
	{name: "-", documentation: doc("(- integer [integer...]) -> Integer", "Negates one integer or subtracts following integers from the first.", "(- 7 2)", "5"), leaky: true, call: pure(subtract)},
	{name: "*", documentation: doc("(* integer...) -> Integer", "Multiplies zero or more integers.", "(* 2 3 4)", "24"), leaky: true, call: pure(multiply)},
	{name: "bool?", documentation: doc("(bool? value) -> Boolean", "Returns whether value is a Boolean.", "(bool? #t)", "#t"), call: pure(isBoolean)},
	{name: "int?", documentation: doc("(int? value) -> Boolean", "Returns whether value is an Integer.", "(int? 1)", "#t"), call: pure(isInteger)},
	{name: "symbol?", documentation: doc("(symbol? value) -> Boolean", "Returns whether value is a Symbol.", "(symbol? 'name)", "#t"), call: pure(isSymbolValue)},
	{name: "int/div", documentation: doc("(int/div dividend divisor) -> Integer", "Divides integers with truncation toward zero. Requires a non-zero divisor.", "(int/div 7 3)", "2"), leaky: true, call: pure(integerDivide)},
	{name: "int/rem", documentation: doc("(int/rem dividend divisor) -> Integer", "Returns the signed remainder of integer division. Requires a non-zero divisor.", "(int/rem 7 3)", "1"), leaky: true, call: pure(integerRemainder)},
	{name: "int/abs", documentation: doc("(int/abs integer) -> Integer", "Returns the non-negative magnitude of an integer.", "(int/abs -4)", "4"), leaky: true, call: pure(integerAbsolute)},
	{name: "int/min", documentation: doc("(int/min integer...) -> Integer", "Returns the smallest supplied integer.", "(int/min 3 1 2)", "1"), leaky: true, call: pure(integerMinimum)},
	{name: "int/max", documentation: doc("(int/max integer...) -> Integer", "Returns the largest supplied integer.", "(int/max 3 1 2)", "3"), leaky: true, call: pure(integerMaximum)},
	{name: "=", documentation: doc("(= value value...) -> Boolean", "Returns whether values have equal types and structural contents.", "(= (list 1 2) (list 1 2))", "#t"), leaky: true, call: pure(equals)},
	{name: "<", documentation: doc("(< integer integer...) -> Boolean", "Returns whether integers are strictly increasing.", "(< 1 2 3)", "#t"), leaky: true, call: pure(lessThan)},
	{name: "<=", documentation: doc("(<= integer integer...) -> Boolean", "Returns whether integers are nondecreasing.", "(<= 1 1 2)", "#t"), leaky: true, call: pure(lessThanOrEqual)},
	{name: ">", documentation: doc("(> integer integer...) -> Boolean", "Returns whether integers are strictly decreasing.", "(> 3 2 1)", "#t"), leaky: true, call: pure(greaterThan)},
	{name: ">=", documentation: doc("(>= integer integer...) -> Boolean", "Returns whether integers are nonincreasing.", "(>= 3 3 1)", "#t"), leaky: true, call: pure(greaterThanOrEqual)},
	{name: "not", documentation: doc("(not boolean) -> Boolean", "Negates a Boolean value.", "(not #t)", "#f"), leaky: true, call: pure(logicalNot)},
	{name: "pair", documentation: doc("(pair first rest) -> Pair", "Creates a pair from two values.", "(pair 1 2)", "(1 . 2)"), leaky: true, call: pure(makePair)},
	{name: "head", documentation: doc("(head pair) -> Value", "Returns the first value of a pair.", "(head (pair 1 2))", "1"), leaky: true, call: pure(head)},
	{name: "tail", documentation: doc("(tail pair) -> Value", "Returns the second value of a pair.", "(tail (pair 1 2))", "2"), leaky: true, call: pure(tail)},
	{name: "list", documentation: doc("(list value...) -> List", "Creates a proper list from its arguments.", "(list 1 2 3)", "(1 2 3)"), leaky: true, call: pure(makeList)},
	{name: "pair?", documentation: doc("(pair? value) -> Boolean", "Returns whether value is a pair.", "(pair? (pair 1 2))", "#t"), call: pure(isPair)},
	{name: "null?", documentation: doc("(null? value) -> Boolean", "Returns whether value is the empty list.", "(null? null)", "#t"), call: pure(isNull)},
	{name: "list?", documentation: doc("(list? value) -> Boolean", "Returns whether value is a proper list.", "(list? (list 1 2))", "#t"), call: pure(isList)},
	{name: "list/length", documentation: doc("(list/length list) -> Integer", "Returns the number of values in a proper list.", "(list/length (list 1 2 3))", "3"), call: pure(listLength)},
	{name: "list/concat", documentation: doc("(list/concat list...) -> List", "Concatenates proper lists.", "(list/concat (list 1 2) (list 3))", "(1 2 3)"), leaky: true, call: pure(concatLists)},
	{name: "list/map", documentation: doc("(list/map function list) -> List", "Applies function to every value in a proper list.", "(list/map (fn (x) (* x x)) (list 1 2 3))", "(1 4 9)"), leaky: true, call: mapValues},
	{name: "list/filter", documentation: doc("(list/filter predicate list) -> List", "Keeps list values whose predicate returns true.", "(list/filter (fn (x) (> x 1)) (list 1 2 3))", "(2 3)"), leaky: true, call: filterValues},
	{name: "list/fold", documentation: doc("(list/fold function initial list) -> Value", "Combines a proper list from left to right with an accumulator.", "(list/fold + 0 (list 1 2 3))", "6"), leaky: true, call: foldValues},
	{name: "string?", documentation: doc("(string? value) -> Boolean", "Returns whether value is a String.", "(string? \"text\")", "#t"), call: pure(isString)},
	{name: "string/length", documentation: doc("(string/length text) -> Integer", "Returns the Unicode code-point length of text.", "(string/length \"hello\")", "5"), call: pure(stringLength)},
	{name: "bytes?", documentation: doc("(bytes? value) -> Boolean", "Returns whether value is Bytes.", "(bytes? (bytes/utf8/encode \"x\"))", "#t"), call: pure(isBytes)},
	{name: "bytes/length", documentation: doc("(bytes/length bytes) -> Integer", "Returns the number of bytes in Bytes.", "(bytes/length (bytes/utf8/encode \"hi\"))", "2"), call: pure(bytesLength)},
	{name: "bytes/concat", documentation: doc("(bytes/concat bytes...) -> Bytes", "Concatenates Bytes values.", "(bytes/hex/encode (bytes/concat (bytes/hex/decode \"aa\") (bytes/hex/decode \"bb\")))", "\"aabb\""), leaky: true, call: pure(bytesConcat)},
	{name: "bytes/slice", documentation: doc("(bytes/slice bytes start end) -> Bytes", "Returns Bytes from inclusive start to exclusive end.", "(bytes/hex/encode (bytes/slice (bytes/hex/decode \"aabbcc\") 1 3))", "\"bbcc\""), leaky: true, call: pure(bytesSlice)},
	{name: "bytes/int/be/encode", documentation: doc("(bytes/int/be/encode integer length) -> Bytes", "Encodes an Integer as signed big-endian two's-complement Bytes of length 1 through 8.", "(bytes/hex/encode (bytes/int/be/encode -2 2))", "\"fffe\""), leaky: true, call: pure(bytesIntBEEncode)},
	{name: "bytes/int/be/decode", documentation: doc("(bytes/int/be/decode bytes) -> Integer", "Decodes signed big-endian two's-complement Bytes of length 1 through 8.", "(bytes/int/be/decode (bytes/hex/decode \"fffe\"))", "-2"), leaky: true, call: pure(bytesIntBEDecode)},
	{name: "bytes/int/le/encode", documentation: doc("(bytes/int/le/encode integer length) -> Bytes", "Encodes an Integer as signed little-endian two's-complement Bytes of length 1 through 8.", "(bytes/hex/encode (bytes/int/le/encode -2 2))", "\"feff\""), leaky: true, call: pure(bytesIntLEEncode)},
	{name: "bytes/int/le/decode", documentation: doc("(bytes/int/le/decode bytes) -> Integer", "Decodes signed little-endian two's-complement Bytes of length 1 through 8.", "(bytes/int/le/decode (bytes/hex/decode \"feff\"))", "-2"), leaky: true, call: pure(bytesIntLEDecode)},
	{name: "bytes/uint/be/encode", documentation: doc("(bytes/uint/be/encode integer length) -> Bytes", "Encodes a non-negative Integer as unsigned big-endian Bytes of length 1 through 8.", "(bytes/hex/encode (bytes/uint/be/encode 258 2))", "\"0102\""), leaky: true, call: pure(bytesUIntBEEncode)},
	{name: "bytes/uint/be/decode", documentation: doc("(bytes/uint/be/decode bytes) -> Integer", "Decodes unsigned big-endian Bytes of length 1 through 8 within the Integer range.", "(bytes/uint/be/decode (bytes/hex/decode \"0102\"))", "258"), leaky: true, call: pure(bytesUIntBEDecode)},
	{name: "bytes/uint/le/encode", documentation: doc("(bytes/uint/le/encode integer length) -> Bytes", "Encodes a non-negative Integer as unsigned little-endian Bytes of length 1 through 8.", "(bytes/hex/encode (bytes/uint/le/encode 258 2))", "\"0201\""), leaky: true, call: pure(bytesUIntLEEncode)},
	{name: "bytes/uint/le/decode", documentation: doc("(bytes/uint/le/decode bytes) -> Integer", "Decodes unsigned little-endian Bytes of length 1 through 8 within the Integer range.", "(bytes/uint/le/decode (bytes/hex/decode \"0201\"))", "258"), leaky: true, call: pure(bytesUIntLEDecode)},
	{name: "bytes/utf8/encode", documentation: doc("(bytes/utf8/encode text) -> Bytes", "Encodes a String as UTF-8 Bytes.", "(bytes/hex/encode (bytes/utf8/encode \"hi\"))", "\"6869\""), leaky: true, call: pure(bytesUTF8Encode)},
	{name: "bytes/utf8/decode", documentation: doc("(bytes/utf8/decode bytes) -> String", "Decodes valid UTF-8 Bytes as a String.", "(bytes/utf8/decode (bytes/hex/decode \"6869\"))", "\"hi\""), leaky: true, call: pure(bytesUTF8Decode)},
	{name: "bytes/hex/encode", documentation: doc("(bytes/hex/encode bytes) -> String", "Encodes Bytes as lower-case hexadecimal text.", "(bytes/hex/encode (bytes/utf8/encode \"hi\"))", "\"6869\""), leaky: true, call: pure(bytesHexEncode)},
	{name: "bytes/hex/decode", documentation: doc("(bytes/hex/decode text) -> Bytes", "Decodes hexadecimal text as Bytes.", "(bytes/length (bytes/hex/decode \"ff00\"))", "2"), leaky: true, call: pure(bytesHexDecode)},
	{name: "bytes/base64/encode", documentation: doc("(bytes/base64/encode bytes) -> String", "Encodes Bytes as standard padded Base64 text.", "(bytes/base64/encode (bytes/utf8/encode \"hi\"))", "\"aGk=\""), leaky: true, call: pure(bytesBase64Encode)},
	{name: "bytes/base64/decode", documentation: doc("(bytes/base64/decode text) -> Bytes", "Decodes standard padded Base64 text as Bytes.", "(bytes/utf8/decode (bytes/base64/decode \"aGk=\"))", "\"hi\""), leaky: true, call: pure(bytesBase64Decode)},
	{name: "bytes/base64url/encode", documentation: doc("(bytes/base64url/encode bytes) -> String", "Encodes Bytes as URL-safe padded Base64 text.", "(bytes/base64url/encode (bytes/utf8/encode \"hi\"))", "\"aGk=\""), leaky: true, call: pure(bytesBase64URLEncode)},
	{name: "bytes/base64url/decode", documentation: doc("(bytes/base64url/decode text) -> Bytes", "Decodes URL-safe padded Base64 text as Bytes.", "(bytes/utf8/decode (bytes/base64url/decode \"aGk=\"))", "\"hi\""), leaky: true, call: pure(bytesBase64URLDecode)},
	{name: "bytes/base64/pad", documentation: doc("(bytes/base64/pad text) -> String", "Normalizes standard or URL-safe Base64 text with padding.", "(bytes/base64/pad \"aGk\")", "\"aGk=\""), leaky: true, call: pure(bytesBase64Pad)},
	{name: "bytes/base64/unpad", documentation: doc("(bytes/base64/unpad text) -> String", "Normalizes standard or URL-safe Base64 text without padding.", "(bytes/base64/unpad \"aGk=\")", "\"aGk\""), leaky: true, call: pure(bytesBase64Unpad)},
	{name: "string/concat", documentation: doc("(string/concat text...) -> String", "Concatenates Strings.", "(string/concat \"a\" \"b\")", "\"ab\""), leaky: true, call: pure(stringConcat)},
	{name: "string/slice", documentation: doc("(string/slice text start end) -> String", "Returns text from inclusive start to exclusive end by Unicode code point.", "(string/slice \"hello\" 1 4)", "\"ell\""), leaky: true, call: pure(stringSlice)},
	{name: "string/trim", documentation: doc("(string/trim text) -> String", "Removes Unicode whitespace from both ends of text.", "(string/trim \"  hi  \")", "\"hi\""), leaky: true, call: pure(stringTrim)},
	{name: "string/lower", documentation: doc("(string/lower text) -> String", "Converts text to locale-independent Unicode lower case.", "(string/lower \"Hi\")", "\"hi\""), leaky: true, call: pure(stringLower)},
	{name: "string/upper", documentation: doc("(string/upper text) -> String", "Converts text to locale-independent Unicode upper case.", "(string/upper \"Hi\")", "\"HI\""), leaky: true, call: pure(stringUpper)},
	{name: "string/contains?", documentation: doc("(string/contains? text substring) -> Boolean", "Returns whether text contains substring.", "(string/contains? \"hello\" \"ell\")", "#t"), call: pure(stringContains)},
	{name: "string/prefix?", documentation: doc("(string/prefix? text prefix) -> Boolean", "Returns whether text starts with prefix.", "(string/prefix? \"hello\" \"he\")", "#t"), call: pure(stringPrefix)},
	{name: "string/suffix?", documentation: doc("(string/suffix? text suffix) -> Boolean", "Returns whether text ends with suffix.", "(string/suffix? \"hello\" \"lo\")", "#t"), call: pure(stringSuffix)},
	{name: "string/split", documentation: doc("(string/split text separator) -> List", "Splits text on a literal separator into a list of Strings.", "(string/split \"a,b\" \",\")", "(\"a\" \"b\")"), leaky: true, call: pure(stringSplit)},
	{name: "string/join", documentation: doc("(string/join strings separator) -> String", "Joins a proper list of Strings with a separator.", "(string/join (list \"a\" \"b\") \",\")", "\"a,b\""), leaky: true, call: pure(stringJoin)},
	{name: "string/replace", documentation: doc("(string/replace text old new) -> String", "Replaces every non-overlapping occurrence of old with new.", "(string/replace \"a-b-a\" \"a\" \"x\")", "\"x-b-x\""), leaky: true, call: pure(stringReplace)},
	{name: "error/throw", documentation: doc("(error/throw value) -> Never", "Raises an Error carrying value.", "(error/value (error/catch (error/throw \"failure\")))", "\"failure\""), call: pure(throwError)},
	{name: "error/catch", documentation: doc("(error/catch expression) -> Error", "Returns an Error raised while evaluating expression.", "(error? (error/catch (error/throw \"failure\")))", "#t"), special: true, call: catchError},
	{name: "error?", documentation: doc("(error? value) -> Boolean", "Returns whether value is an Error.", "(error? (error/catch (error/throw 1)))", "#t"), call: pure(isError)},
	{name: "error/value", documentation: doc("(error/value error) -> Value", "Returns the value carried by an Error.", "(error/value (error/catch (error/throw 1)))", "1"), leaky: true, call: pure(errorValueOf)},
	{name: "assert", documentation: doc("(assert condition [value]) -> Null | Value", "Returns null or value when condition is true, otherwise raises an Error.", "(assert #t 7)", "7"), special: true, call: assertValue},
	{name: "secret?", documentation: doc("(secret? value) -> Boolean", "Returns whether value is secret-tainted.", "(secret? (secret/mark \"value\"))", "#t"), call: pure(isSecret)},
	{name: "secret/mark", documentation: doc("(secret/mark value) -> Value", "Marks value as secret-tainted.", "(secret? (secret/mark \"value\"))", "#t"), call: pure(markSecret)},
	{name: "help", documentation: doc("(help value-or-name) -> String", "Returns public help text. Quote a name to look up a visible binding or evaluator form; other expressions are evaluated before being described.", `(string/contains? (help 'fn) "(fn (parameter...) body) -> Function")`, "#t"), call: helpValue},
	{name: "help/document", documentation: doc("(help/document value text) -> Value", "Returns value with public help text attached.", "(help (help/document 1 \"count\"))", "\"count\""), call: pure(helpDocument)},
	{name: "help/env", documentation: doc("(help/env term...) -> List", "Returns visible lexical binding names matching every term.", "(list? (help/env \"int?\"))", "#t"), call: helpEnv},
	{name: "help/search", documentation: doc("(help/search term...) -> List", "Returns symbols for values and forms available in the environment with help text that matches every search term.", "(list? (help/search \"integer\"))", "#t"), call: helpSearch},
	{name: "import/restrict", documentation: doc("(import/restrict (@reference...) body) -> Value", "Evaluates body with imports limited to the listed module references.", "(import/restrict (@native:time/v1) (import/search))", "(@native:time/v1)"), special: true, call: importRestrict},
}

func (env *environment) bind(name string, value Expr) {
	bound := value
	env.values[name] = &bound
}

func add(_ *evaluator, arguments []Expr) (error, Expr) {
	var total int64
	for _, argument := range arguments {
		err, value := requireInteger(argument)
		if err != nil {
			return err, nil
		}
		if (value > 0 && total > math.MaxInt64-value) || (value < 0 && total < math.MinInt64-value) {
			return expressionError("integer overflow"), nil
		}
		total += value
	}
	return nil, integer(total)
}

func subtract(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) == 0 {
		return expressionError("- requires at least one argument"), nil
	}
	err, total := requireInteger(arguments[0])
	if err != nil {
		return err, nil
	}
	if len(arguments) == 1 {
		if total == math.MinInt64 {
			return expressionError("integer overflow"), nil
		}
		return nil, integer(-total)
	}
	for _, argument := range arguments[1:] {
		err, value := requireInteger(argument)
		if err != nil {
			return err, nil
		}
		if (value > 0 && total < math.MinInt64+value) || (value < 0 && total > math.MaxInt64+value) {
			return expressionError("integer overflow"), nil
		}
		total -= value
	}
	return nil, integer(total)
}

func multiply(_ *evaluator, arguments []Expr) (error, Expr) {
	total := int64(1)
	for _, argument := range arguments {
		err, value := requireInteger(argument)
		if err != nil {
			return err, nil
		}
		if multiplicationOverflows(total, value) {
			return expressionError("integer overflow"), nil
		}
		total *= value
	}
	return nil, integer(total)
}

func isBoolean(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("bool? requires one argument"), nil
	}
	base, _ := unwrap(arguments[0])
	_, ok := base.(*booleanExpr)
	return nil, boolean(ok)
}

func isInteger(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("int? requires one argument"), nil
	}
	base, _ := unwrap(arguments[0])
	_, ok := base.(*integerExpr)
	return nil, boolean(ok)
}

func isSymbolValue(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("symbol? requires one argument"), nil
	}
	base, _ := unwrap(arguments[0])
	_, ok := base.(*symbolExpr)
	return nil, boolean(ok)
}

func isSecret(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("secret? requires one argument"), nil
	}
	return nil, boolean(hasSecret(arguments[0]))
}

func markSecret(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("secret/mark requires one argument"), nil
	}
	return nil, withSecret(arguments[0])
}

func integerDivide(_ *evaluator, arguments []Expr) (error, Expr) {
	err, dividend, divisor := integerOperands(arguments, "int/div")
	if err != nil {
		return err, nil
	}
	if divisor == 0 {
		return expressionError("int/div requires a non-zero divisor"), nil
	}
	if dividend == math.MinInt64 && divisor == -1 {
		return expressionError("integer overflow"), nil
	}
	return nil, integer(dividend / divisor)
}

func integerRemainder(_ *evaluator, arguments []Expr) (error, Expr) {
	err, dividend, divisor := integerOperands(arguments, "int/rem")
	if err != nil {
		return err, nil
	}
	if divisor == 0 {
		return expressionError("int/rem requires a non-zero divisor"), nil
	}
	return nil, integer(dividend % divisor)
}

func integerAbsolute(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("int/abs requires one integer"), nil
	}
	err, value := requireInteger(arguments[0])
	if err != nil {
		return err, nil
	}
	if value == math.MinInt64 {
		return expressionError("integer overflow"), nil
	}
	if value < 0 {
		value = -value
	}
	return nil, integer(value)
}

func integerMinimum(_ *evaluator, arguments []Expr) (error, Expr) {
	return integerExtremum(arguments, "int/min", func(candidate int64, current int64) bool {
		return candidate < current
	})
}

func integerMaximum(_ *evaluator, arguments []Expr) (error, Expr) {
	return integerExtremum(arguments, "int/max", func(candidate int64, current int64) bool {
		return candidate > current
	})
}

func equals(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) < 2 {
		return expressionError("= requires at least two arguments"), nil
	}
	for index := 1; index < len(arguments); index++ {
		if !equal(arguments[index-1], arguments[index]) {
			return nil, boolean(false)
		}
	}
	return nil, boolean(true)
}

func lessThan(_ *evaluator, arguments []Expr) (error, Expr) {
	return compareIntegers(arguments, func(left int64, right int64) bool { return left < right }, "<")
}

func lessThanOrEqual(_ *evaluator, arguments []Expr) (error, Expr) {
	return compareIntegers(arguments, func(left int64, right int64) bool { return left <= right }, "<=")
}

func greaterThan(_ *evaluator, arguments []Expr) (error, Expr) {
	return compareIntegers(arguments, func(left int64, right int64) bool { return left > right }, ">")
}

func greaterThanOrEqual(_ *evaluator, arguments []Expr) (error, Expr) {
	return compareIntegers(arguments, func(left int64, right int64) bool { return left >= right }, ">=")
}

func compareIntegers(arguments []Expr, compare func(int64, int64) bool, name string) (error, Expr) {
	if len(arguments) < 2 {
		return expressionError("%s requires at least two arguments", name), nil
	}
	err, previous := requireInteger(arguments[0])
	if err != nil {
		return err, nil
	}
	for _, argument := range arguments[1:] {
		err, current := requireInteger(argument)
		if err != nil {
			return err, nil
		}
		if !compare(previous, current) {
			return nil, boolean(false)
		}
		previous = current
	}
	return nil, boolean(true)
}

func logicalNot(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("not requires one argument"), nil
	}
	err, value := requireBoolean(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, boolean(!value)
}

func makePair(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError("pair requires two arguments"), nil
	}
	return nil, pairValue(arguments[0], arguments[1])
}

func head(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("head requires one argument"), nil
	}
	base, _ := unwrap(arguments[0])
	pair, ok := base.(*pair)
	if !ok {
		return expressionError("head requires a pair"), nil
	}
	return nil, pair.first
}

func tail(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("tail requires one argument"), nil
	}
	base, _ := unwrap(arguments[0])
	pair, ok := base.(*pair)
	if !ok {
		return expressionError("tail requires a pair"), nil
	}
	return nil, pair.rest
}

func makeList(_ *evaluator, arguments []Expr) (error, Expr) {
	return nil, list(arguments)
}

func isPair(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("pair? requires one argument"), nil
	}
	base, _ := unwrap(arguments[0])
	_, ok := base.(*pair)
	return nil, boolean(ok)
}

func isNull(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("null? requires one argument"), nil
	}
	base, _ := unwrap(arguments[0])
	_, ok := base.(*nullExpr)
	return nil, boolean(ok)
}

func isList(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("list? requires one argument"), nil
	}
	return nil, boolean(isProperList(arguments[0]))
}

func listLength(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("list/length requires one argument"), nil
	}
	err, values := expressions(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, integer(int64(len(values)))
}

func concatLists(_ *evaluator, arguments []Expr) (error, Expr) {
	var result []Expr
	for _, argument := range arguments {
		err, values := expressions(argument)
		if err != nil {
			return err, nil
		}
		result = append(result, values...)
	}
	return nil, list(result)
}

func mapValues(evaluator *evaluator, env *environment, arguments []Expr) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError("list/map requires a function and list"), nil
	}
	err, values := expressions(arguments[1])
	if err != nil {
		return err, nil
	}
	result := make([]Expr, 0, len(values))
	for _, value := range values {
		err, mapped := evaluator.call(arguments[0], env, []Expr{value})
		if err != nil {
			return err, nil
		}
		result = append(result, mapped)
	}
	return nil, list(result)
}

func filterValues(evaluator *evaluator, env *environment, arguments []Expr) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError("list/filter requires a function and list"), nil
	}
	err, values := expressions(arguments[1])
	if err != nil {
		return err, nil
	}
	result := make([]Expr, 0, len(values))
	for _, value := range values {
		err, keep := evaluator.call(arguments[0], env, []Expr{value})
		if err != nil {
			return err, nil
		}
		err, include := requireBoolean(keep)
		if err != nil {
			return err, nil
		}
		if include {
			result = append(result, value)
		}
	}
	return nil, list(result)
}

func foldValues(evaluator *evaluator, env *environment, arguments []Expr) (error, Expr) {
	if len(arguments) != 3 {
		return expressionError("list/fold requires a function, initial value, and list"), nil
	}
	err, values := expressions(arguments[2])
	if err != nil {
		return err, nil
	}
	result := arguments[1]
	for _, value := range values {
		err, result = evaluator.call(arguments[0], env, []Expr{result, value})
		if err != nil {
			return err, nil
		}
	}
	return nil, result
}

func requireInteger(expr Expr) (error, int64) {
	base, _ := unwrap(expr)
	value, ok := base.(*integerExpr)
	if !ok {
		return expressionError("expected an integer, got %s", expr.String()), 0
	}
	return nil, value.value
}

func integerOperands(arguments []Expr, name string) (error, int64, int64) {
	if len(arguments) != 2 {
		return expressionError("%s requires two integers", name), 0, 0
	}
	err, dividend := requireInteger(arguments[0])
	if err != nil {
		return err, 0, 0
	}
	err, divisor := requireInteger(arguments[1])
	if err != nil {
		return err, 0, 0
	}
	return nil, dividend, divisor
}

func integerExtremum(arguments []Expr, name string, prefer func(candidate int64, current int64) bool) (error, Expr) {
	if len(arguments) == 0 {
		return expressionError("%s requires at least one integer", name), nil
	}
	err, result := requireInteger(arguments[0])
	if err != nil {
		return err, nil
	}
	for _, argument := range arguments[1:] {
		err, value := requireInteger(argument)
		if err != nil {
			return err, nil
		}
		if prefer(value, result) {
			result = value
		}
	}
	return nil, integer(result)
}

func isProperList(expr Expr) bool {
	for {
		base, _ := unwrap(expr)
		if pair, ok := base.(*pair); ok {
			expr = pair.rest
			continue
		}
		_, ok := base.(*nullExpr)
		return ok
	}
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
