package lisp

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"unicode/utf8"
)

func isBytes(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "bytes? requires one argument"), Expr{}
	}
	return nil, boolean(arguments[0].kind == exprBytes, Span{})
}

func bytesLength(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "bytes/length requires one argument"), Expr{}
	}
	err, value := requireBytes(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	return nil, integer(int64(len(value)), Span{})
}

func bytesConcat(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	var builder strings.Builder
	for _, argument := range arguments {
		err, value := requireBytes(argument, span)
		if err != nil {
			return err, Expr{}
		}
		builder.WriteString(value)
	}
	return nil, bytesValue(builder.String(), Span{})
}

func bytesSlice(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 3 {
		return expressionError(span, "bytes/slice requires Bytes, start, and end"), Expr{}
	}
	err, value := requireBytes(arguments[0], span)
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
	if start < 0 || end < start || end > int64(len(value)) {
		return expressionError(span, "bytes/slice indices are out of range"), Expr{}
	}
	return nil, bytesValue(value[start:end], Span{})
}

func bytesUTF8Encode(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "bytes/utf8/encode requires one string"), Expr{}
	}
	err, value := requireString(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	return nil, bytesValue(value, Span{})
}

func bytesUTF8Decode(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "bytes/utf8/decode requires one Bytes value"), Expr{}
	}
	err, value := requireBytes(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	if !utf8.ValidString(value) {
		return expressionError(span, "bytes/utf8/decode requires valid UTF-8 Bytes"), Expr{}
	}
	return nil, stringValue(value, Span{})
}

func bytesHexEncode(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "bytes/hex/encode requires one Bytes value"), Expr{}
	}
	err, value := requireBytes(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	return nil, stringValue(hex.EncodeToString([]byte(value)), Span{})
}

func bytesHexDecode(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "bytes/hex/decode requires one string"), Expr{}
	}
	err, value := requireString(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	decoded, decodeErr := hex.DecodeString(value)
	if decodeErr != nil {
		return expressionError(span, "bytes/hex/decode requires hexadecimal text"), Expr{}
	}
	return nil, bytesValue(string(decoded), Span{})
}

func bytesBase64Encode(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "bytes/base64/encode requires one Bytes value"), Expr{}
	}
	err, value := requireBytes(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	return nil, stringValue(base64.StdEncoding.EncodeToString([]byte(value)), Span{})
}

func bytesBase64Decode(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "bytes/base64/decode requires one string"), Expr{}
	}
	err, value := requireString(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	decoded, decodeErr := base64.StdEncoding.Strict().DecodeString(value)
	if decodeErr != nil {
		return expressionError(span, "bytes/base64/decode requires standard padded Base64 text"), Expr{}
	}
	return nil, bytesValue(string(decoded), Span{})
}

func bytesBase64URLEncode(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "bytes/base64url/encode requires one Bytes value"), Expr{}
	}
	err, value := requireBytes(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	return nil, stringValue(base64.URLEncoding.EncodeToString([]byte(value)), Span{})
}

func bytesBase64URLDecode(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "bytes/base64url/decode requires one string"), Expr{}
	}
	err, value := requireString(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	decoded, decodeErr := base64.URLEncoding.Strict().DecodeString(value)
	if decodeErr != nil {
		return expressionError(span, "bytes/base64url/decode requires URL-safe padded Base64 text"), Expr{}
	}
	return nil, bytesValue(string(decoded), Span{})
}

func bytesBase64Pad(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "bytes/base64/pad requires one string"), Expr{}
	}
	err, value := requireString(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	err, padded := normalizeBase64Padding(value, true)
	if err != nil {
		return expressionError(span, "bytes/base64/pad requires valid Base64 text"), Expr{}
	}
	return nil, stringValue(padded, Span{})
}

func bytesBase64Unpad(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "bytes/base64/unpad requires one string"), Expr{}
	}
	err, value := requireString(arguments[0], span)
	if err != nil {
		return err, Expr{}
	}
	err, unpadded := normalizeBase64Padding(value, false)
	if err != nil {
		return expressionError(span, "bytes/base64/unpad requires valid Base64 text"), Expr{}
	}
	return nil, stringValue(unpadded, Span{})
}

func normalizeBase64Padding(value string, padded bool) (error, string) {
	paddingStart := len(value)
	standard := false
	url := false
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character == '=' {
			paddingStart = index
			break
		}
		switch {
		case 'A' <= character && character <= 'Z', 'a' <= character && character <= 'z', '0' <= character && character <= '9':
		case character == '+' || character == '/':
			standard = true
		case character == '-' || character == '_':
			url = true
		default:
			return expressionError(Span{}, "invalid Base64 character"), ""
		}
	}
	if standard && url {
		return expressionError(Span{}, "mixed Base64 alphabets"), ""
	}
	for _, character := range value[paddingStart:] {
		if character != '=' {
			return expressionError(Span{}, "invalid Base64 padding"), ""
		}
	}

	rawLength := paddingStart
	if rawLength%4 == 1 {
		return expressionError(Span{}, "invalid Base64 length"), ""
	}
	expectedPadding := (4 - rawLength%4) % 4
	actualPadding := len(value) - rawLength
	if actualPadding != 0 && actualPadding != expectedPadding {
		return expressionError(Span{}, "invalid Base64 padding"), ""
	}
	normalized := value[:rawLength] + strings.Repeat("=", expectedPadding)
	if url {
		_, err := base64.URLEncoding.Strict().DecodeString(normalized)
		if err != nil {
			return err, ""
		}
	} else {
		_, err := base64.StdEncoding.Strict().DecodeString(normalized)
		if err != nil {
			return err, ""
		}
	}
	if padded {
		return nil, normalized
	}
	return nil, value[:rawLength]
}

func requireBytes(expr Expr, span Span) (error, string) {
	if expr.kind != exprBytes {
		return expressionError(span, "expected Bytes, got %s", expr.String()), ""
	}
	return nil, expr.bytes
}
