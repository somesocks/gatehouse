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
	"Decodes JSON text into tagged JSON nodes. Objects and arrays contain their members directly; numbers retain their JSON text.",
	"(import (json @native:json/v1) (json/decode \"{\\\"ok\\\":true}\"))",
	"(json/object (\"ok\" (json/boolean #t)))",
)

var nativeJSONEncodeDocumentation = doc(
	"(json/encode json) -> String",
	"Encodes a tagged JSON node as JSON text.",
	"(import (json @native:json/v1) (json/encode '(json/array (json/number \"1\") (json/null))))",
	"\"[1,null]\"",
)

func nativeJSONModule() Expr {
	decode := withHelp(&builtin{leaky: true, call: pure(nativeJSONDecode)}, nativeJSONDecodeDocumentation.text())
	encode := withHelp(&builtin{leaky: true, call: pure(nativeJSONEncode)}, nativeJSONEncodeDocumentation.text())
	exports := list([]Expr{
		pairValue(symbol("decode"), decode),
		pairValue(symbol("encode"), encode),
	})
	return list([]Expr{symbol("quote"), exports})
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
				members = append(members, list([]Expr{stringValue(keyText), value}))
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
		decoder := json.NewDecoder(strings.NewReader(value))
		decoder.UseNumber()
		var decoded any
		if err := decoder.Decode(&decoded); err != nil {
			return expressionError("json/number requires JSON number text"), ""
		}
		if _, ok := decoded.(json.Number); !ok {
			return expressionError("json/number requires JSON number text"), ""
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return expressionError("json/number requires JSON number text"), ""
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
			err, pair := expressions(member)
			if err != nil || len(pair) != 2 {
				return expressionError("json/object members require a String key and JSON value"), ""
			}
			err, key := requireString(pair[0])
			if err != nil {
				return err, ""
			}
			err, value := nativeJSONEncodeValue(pair[1])
			if err != nil {
				return err, ""
			}
			if index > 0 {
				builder.WriteByte(',')
			}
			encodedKey, _ := json.Marshal(key)
			builder.Write(encodedKey)
			builder.WriteByte(':')
			builder.WriteString(value)
		}
		builder.WriteByte('}')
		return nil, builder.String()
	default:
		return expressionError("json/encode requires a supported JSON tag"), ""
	}
}
