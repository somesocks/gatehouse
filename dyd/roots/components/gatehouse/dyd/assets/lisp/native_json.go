package lisp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const nativeJSONID = "native:json/v1"

var nativeJSONDecodeDocumentation = doc(
	"(json/decode text) -> JSON",
	"Decodes JSON text into tagged JSON values created by json/null, json/boolean, json/string, json/number, json/array, and json/object. Numbers retain their JSON text.",
	"(json/decode \"{\\\"ok\\\":true}\")",
	"(json/object (\"ok\" json/boolean #t))",
)

var nativeJSONEncodeDocumentation = doc(
	"(json/encode json) -> String",
	"Encodes a JSON value created by json/null, json/boolean, json/string, json/number, json/array, or json/object as JSON text.",
	"(json/encode (json/array (json/number \"1\") (json/null)))",
	"\"[1,null]\"",
)

func nativeJSONModule() Expr {
	decode := withHelp(&builtin{leaky: true, call: pure(nativeJSONDecode)}, nativeJSONDecodeDocumentation.text())
	encode := withHelp(&builtin{leaky: true, call: pure(nativeJSONEncode)}, nativeJSONEncodeDocumentation.text())
	exports := list([]Expr{
		pairValue(symbol("null"), withHelp(&builtin{leaky: true, call: pure(nativeJSONNull)}, doc("(json/null) -> JSON", "Creates the JSON null value.", "(json/null)", "(json/null)").text())),
		pairValue(symbol("null?"), withHelp(&builtin{call: pure(nativeJSONNullQ)}, doc("(json/null? value) -> Boolean", "Returns whether value is a JSON Null.", "(json/null? (json/null))", "#t").text())),
		pairValue(symbol("boolean"), withHelp(&builtin{leaky: true, call: pure(nativeJSONBoolean)}, doc("(json/boolean boolean) -> JSON", "Creates a JSON Boolean value.", "(json/boolean #t)", "(json/boolean #t)").text())),
		pairValue(symbol("boolean?"), withHelp(&builtin{call: pure(nativeJSONBooleanQ)}, doc("(json/boolean? value) -> Boolean", "Returns whether value is a JSON Boolean.", "(json/boolean? (json/boolean #t))", "#t").text())),
		pairValue(symbol("string"), withHelp(&builtin{leaky: true, call: pure(nativeJSONString)}, doc("(json/string text) -> JSON", "Creates a JSON String value.", "(json/string \"Gatehouse\")", "(json/string \"Gatehouse\")").text())),
		pairValue(symbol("string?"), withHelp(&builtin{call: pure(nativeJSONStringQ)}, doc("(json/string? value) -> Boolean", "Returns whether value is a JSON String.", "(json/string? (json/string \"Gatehouse\"))", "#t").text())),
		pairValue(symbol("number"), withHelp(&builtin{leaky: true, call: pure(nativeJSONNumber)}, doc("(json/number text) -> JSON", "Creates a JSON Number from valid JSON number text.", "(json/number \"1.25\")", "(json/number \"1.25\")").text())),
		pairValue(symbol("number?"), withHelp(&builtin{call: pure(nativeJSONNumberQ)}, doc("(json/number? value) -> Boolean", "Returns whether value is a JSON Number.", "(json/number? (json/number \"1.25\"))", "#t").text())),
		pairValue(symbol("array"), withHelp(&builtin{leaky: true, call: pure(nativeJSONArray)}, doc("(json/array json...) -> JSON", "Creates a JSON Array from JSON values.", "(json/array (json/string \"a\") (json/null))", "(json/array (json/string \"a\") (json/null))").text())),
		pairValue(symbol("array?"), withHelp(&builtin{call: pure(nativeJSONArrayQ)}, doc("(json/array? value) -> Boolean", "Returns whether value is a JSON Array.", "(json/array? (json/array))", "#t").text())),
		pairValue(symbol("object"), withHelp(&builtin{leaky: true, call: pure(nativeJSONObject)}, doc("(json/object (pair text json)...) -> JSON", "Creates a JSON Object from String-keyed JSON value pairs.", "(json/object (pair \"name\" (json/string \"Gatehouse\")))", "(json/object (\"name\" . (json/string \"Gatehouse\")))").text())),
		pairValue(symbol("object?"), withHelp(&builtin{call: pure(nativeJSONObjectQ)}, doc("(json/object? value) -> Boolean", "Returns whether value is a JSON Object.", "(json/object? (json/object))", "#t").text())),
		pairValue(symbol("decode"), decode),
		pairValue(symbol("encode"), encode),
	})
	return list([]Expr{symbol("quote"), exports})
}

func nativeJSONNull(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 0 {
		return expressionError("json/null requires no values"), nil
	}
	return nil, list([]Expr{symbol("json/null")})
}

func nativeJSONNullQ(_ *evaluator, arguments []Expr) (error, Expr) {
	return nativeJSONTypeQ(arguments, "json/null", "json/null?")
}

func nativeJSONBoolean(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("json/boolean requires one Boolean"), nil
	}
	err, value := requireBoolean(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, list([]Expr{symbol("json/boolean"), boolean(value)})
}

func nativeJSONBooleanQ(_ *evaluator, arguments []Expr) (error, Expr) {
	return nativeJSONTypeQ(arguments, "json/boolean", "json/boolean?")
}

func nativeJSONString(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("json/string requires one String"), nil
	}
	err, value := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, list([]Expr{symbol("json/string"), stringValue(value)})
}

func nativeJSONStringQ(_ *evaluator, arguments []Expr) (error, Expr) {
	return nativeJSONTypeQ(arguments, "json/string", "json/string?")
}

func nativeJSONNumber(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("json/number requires one String"), nil
	}
	err, value := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	if err := nativeJSONNumberText(value); err != nil {
		return err, nil
	}
	return nil, list([]Expr{symbol("json/number"), stringValue(value)})
}

func nativeJSONNumberQ(_ *evaluator, arguments []Expr) (error, Expr) {
	return nativeJSONTypeQ(arguments, "json/number", "json/number?")
}

func nativeJSONArray(_ *evaluator, arguments []Expr) (error, Expr) {
	values := []Expr{symbol("json/array")}
	for _, value := range arguments {
		if err, _ := nativeJSONEncodeValue(value); err != nil {
			return err, nil
		}
		values = append(values, value)
	}
	return nil, list(values)
}

func nativeJSONArrayQ(_ *evaluator, arguments []Expr) (error, Expr) {
	return nativeJSONTypeQ(arguments, "json/array", "json/array?")
}

func nativeJSONObject(_ *evaluator, arguments []Expr) (error, Expr) {
	values := []Expr{symbol("json/object")}
	for _, member := range arguments {
		if err := nativeJSONObjectMember(member); err != nil {
			return err, nil
		}
		values = append(values, member)
	}
	return nil, list(values)
}

func nativeJSONObjectQ(_ *evaluator, arguments []Expr) (error, Expr) {
	return nativeJSONTypeQ(arguments, "json/object", "json/object?")
}

func nativeJSONTypeQ(arguments []Expr, tag string, name string) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("%s requires one argument", name), nil
	}
	err, values := expressions(arguments[0])
	if err != nil || len(values) == 0 || !isSymbol(values[0], tag) {
		return nil, boolean(false)
	}
	if err, _ := nativeJSONEncodeValue(arguments[0]); err != nil {
		return nil, boolean(false)
	}
	return nil, boolean(true)
}

func nativeJSONDecode(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("json/decode requires one String"), nil
	}
	err, text := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	err, value := nativeJSONDecodeValue(decoder)
	if err != nil {
		return expressionError("json/decode requires valid JSON: %v", err), nil
	}
	if _, err := decoder.Token(); err != io.EOF {
		return expressionError("json/decode requires one JSON value"), nil
	}
	if hasSecret(arguments[0]) {
		value = withSecret(value)
	}
	return nil, value
}

func nativeJSONDecodeValue(decoder *json.Decoder) (error, Expr) {
	token, err := decoder.Token()
	if err != nil {
		return err, nil
	}
	switch token := token.(type) {
	case nil:
		return nil, list([]Expr{symbol("json/null")})
	case bool:
		return nil, list([]Expr{symbol("json/boolean"), boolean(token)})
	case string:
		return nil, list([]Expr{symbol("json/string"), stringValue(token)})
	case json.Number:
		return nil, list([]Expr{symbol("json/number"), stringValue(string(token))})
	case json.Delim:
		switch token {
		case '[':
			values := []Expr{symbol("json/array")}
			for decoder.More() {
				err, value := nativeJSONDecodeValue(decoder)
				if err != nil {
					return err, nil
				}
				values = append(values, value)
			}
			if _, err := decoder.Token(); err != nil {
				return err, nil
			}
			return nil, list(values)
		case '{':
			members := []Expr{symbol("json/object")}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err, nil
				}
				keyText, ok := key.(string)
				if !ok {
					return fmt.Errorf("object key is not a string"), nil
				}
				err, value := nativeJSONDecodeValue(decoder)
				if err != nil {
					return err, nil
				}
			members = append(members, pairValue(stringValue(keyText), value))
			}
			if _, err := decoder.Token(); err != nil {
				return err, nil
			}
			return nil, list(members)
		}
	}
	return fmt.Errorf("unsupported JSON token"), nil
}

func nativeJSONEncode(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("json/encode requires one JSON node"), nil
	}
	err, encoded := nativeJSONEncodeValue(arguments[0])
	if err != nil {
		return err, nil
	}
	result := Expr(stringValue(encoded))
	if hasSecret(arguments[0]) {
		result = withSecret(result)
	}
	return nil, result
}

func nativeJSONEncodeValue(node Expr) (error, string) {
	err, values := expressions(node)
	if err != nil || len(values) == 0 {
		return expressionError("json/encode requires a tagged JSON node"), ""
	}
	base, _ := unwrap(values[0])
	tag, ok := base.(*symbolExpr)
	if !ok {
		return expressionError("json/encode requires a JSON tag"), ""
	}
	switch tag.value {
	case "json/null":
		if len(values) != 1 {
			return expressionError("json/null requires no values"), ""
		}
		return nil, "null"
	case "json/boolean":
		if len(values) != 2 {
			return expressionError("json/boolean requires one Boolean"), ""
		}
		err, value := requireBoolean(values[1])
		if err != nil {
			return err, ""
		}
		if value {
			return nil, "true"
		}
		return nil, "false"
	case "json/string":
		if len(values) != 2 {
			return expressionError("json/string requires one String"), ""
		}
		err, value := requireString(values[1])
		if err != nil {
			return err, ""
		}
		encoded, _ := json.Marshal(value)
		return nil, string(encoded)
	case "json/number":
		if len(values) != 2 {
			return expressionError("json/number requires one String"), ""
		}
		err, value := requireString(values[1])
		if err != nil {
			return err, ""
		}
		if err := nativeJSONNumberText(value); err != nil {
			return err, ""
		}
		return nil, value
	case "json/array":
		encoded := make([]string, len(values)-1)
		for index, value := range values[1:] {
			err, item := nativeJSONEncodeValue(value)
			if err != nil {
				return err, ""
			}
			encoded[index] = item
		}
		return nil, "[" + strings.Join(encoded, ",") + "]"
	case "json/object":
		var builder bytes.Buffer
		builder.WriteByte('{')
		for index, member := range values[1:] {
			err, key, value := nativeJSONObjectMemberValues(member)
			if err != nil {
				return err, ""
			}
			err, encodedValue := nativeJSONEncodeValue(value)
			if err != nil {
				return err, ""
			}
			if index > 0 {
				builder.WriteByte(',')
			}
			encodedKey, _ := json.Marshal(key)
			builder.Write(encodedKey)
			builder.WriteByte(':')
			builder.WriteString(encodedValue)
		}
		builder.WriteByte('}')
		return nil, builder.String()
	default:
		return expressionError("json/encode requires a supported JSON tag"), ""
	}
}

func nativeJSONNumberText(value string) error {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return expressionError("json/number requires JSON number text")
	}
	if _, ok := decoded.(json.Number); !ok {
		return expressionError("json/number requires JSON number text")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return expressionError("json/number requires JSON number text")
	}
	return nil
}

func nativeJSONObjectMember(member Expr) error {
	err, _, _ := nativeJSONObjectMemberValues(member)
	return err
}

func nativeJSONObjectMemberValues(member Expr) (error, string, Expr) {
	base, _ := unwrap(member)
	pair, ok := base.(*pair)
	if !ok {
		return expressionError("json/object members require a String key and JSON value pair"), "", nil
	}
	err, key := requireString(pair.first)
	if err != nil {
		return expressionError("json/object members require a String key and JSON value pair"), "", nil
	}
	if err, _ := nativeJSONEncodeValue(pair.rest); err != nil {
		return err, "", nil
	}
	return nil, key, pair.rest
}
