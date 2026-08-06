package lisp

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"unicode/utf8"
)

func isBytes(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("bytes? requires one argument"), nil
	}
	base, _ := unwrap(arguments[0])
	_, ok := base.(*bytesExpr)
	return nil, boolean(ok)
}
func bytesLength(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("bytes/length requires one argument"), nil
	}
	err, value := requireBytes(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, integer(int64(len(value)))
}
func bytesConcat(_ *evaluator, arguments []Expr) (error, Expr) {
	var builder strings.Builder
	for _, argument := range arguments {
		err, value := requireBytes(argument)
		if err != nil {
			return err, nil
		}
		builder.WriteString(value)
	}
	return nil, bytesValue(builder.String())
}
func bytesSlice(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 3 {
		return expressionError("bytes/slice requires Bytes, start, and end"), nil
	}
	err, value := requireBytes(arguments[0])
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
	if start < 0 || end < start || end > int64(len(value)) {
		return expressionError("bytes/slice indices are out of range"), nil
	}
	return nil, bytesValue(value[start:end])
}
func bytesUTF8Encode(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("bytes/utf8/encode requires one string"), nil
	}
	err, value := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, bytesValue(value)
}
func bytesUTF8Decode(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("bytes/utf8/decode requires one Bytes value"), nil
	}
	err, value := requireBytes(arguments[0])
	if err != nil {
		return err, nil
	}
	if !utf8.ValidString(value) {
		return expressionError("bytes/utf8/decode requires valid UTF-8 Bytes"), nil
	}
	return nil, stringValue(value)
}
func bytesHexEncode(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("bytes/hex/encode requires one Bytes value"), nil
	}
	err, value := requireBytes(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, stringValue(hex.EncodeToString([]byte(value)))
}
func bytesHexDecode(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("bytes/hex/decode requires one string"), nil
	}
	err, value := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	decoded, decodeErr := hex.DecodeString(value)
	if decodeErr != nil {
		return expressionError("bytes/hex/decode requires hexadecimal text"), nil
	}
	return nil, bytesValue(string(decoded))
}
func bytesBase64Encode(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("bytes/base64/encode requires one Bytes value"), nil
	}
	err, value := requireBytes(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, stringValue(base64.StdEncoding.EncodeToString([]byte(value)))
}
func bytesBase64Decode(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("bytes/base64/decode requires one string"), nil
	}
	err, value := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	decoded, decodeErr := base64.StdEncoding.Strict().DecodeString(value)
	if decodeErr != nil {
		return expressionError("bytes/base64/decode requires standard padded Base64 text"), nil
	}
	return nil, bytesValue(string(decoded))
}
func bytesBase64URLEncode(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("bytes/base64url/encode requires one Bytes value"), nil
	}
	err, value := requireBytes(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, stringValue(base64.URLEncoding.EncodeToString([]byte(value)))
}
func bytesBase64URLDecode(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("bytes/base64url/decode requires one string"), nil
	}
	err, value := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	decoded, decodeErr := base64.URLEncoding.Strict().DecodeString(value)
	if decodeErr != nil {
		return expressionError("bytes/base64url/decode requires URL-safe padded Base64 text"), nil
	}
	return nil, bytesValue(string(decoded))
}
func bytesBase64Pad(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("bytes/base64/pad requires one string"), nil
	}
	err, value := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	err, padded := normalizeBase64Padding(value, true)
	if err != nil {
		return expressionError("bytes/base64/pad requires valid Base64 text"), nil
	}
	return nil, stringValue(padded)
}
func bytesBase64Unpad(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("bytes/base64/unpad requires one string"), nil
	}
	err, value := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	err, unpadded := normalizeBase64Padding(value, false)
	if err != nil {
		return expressionError("bytes/base64/unpad requires valid Base64 text"), nil
	}
	return nil, stringValue(unpadded)
}

func normalizeBase64Padding(value string, padded bool) (error, string) {
	paddingStart, standard, url := len(value), false, false
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
			return expressionError("invalid Base64 character"), ""
		}
	}
	if standard && url {
		return expressionError("mixed Base64 alphabets"), ""
	}
	for _, character := range value[paddingStart:] {
		if character != '=' {
			return expressionError("invalid Base64 padding"), ""
		}
	}
	rawLength := paddingStart
	if rawLength%4 == 1 {
		return expressionError("invalid Base64 length"), ""
	}
	expectedPadding := (4 - rawLength%4) % 4
	actualPadding := len(value) - rawLength
	if actualPadding != 0 && actualPadding != expectedPadding {
		return expressionError("invalid Base64 padding"), ""
	}
	normalized := value[:rawLength] + strings.Repeat("=", expectedPadding)
	var err error
	if url {
		_, err = base64.URLEncoding.Strict().DecodeString(normalized)
	} else {
		_, err = base64.StdEncoding.Strict().DecodeString(normalized)
	}
	if err != nil {
		return err, ""
	}
	if padded {
		return nil, normalized
	}
	return nil, value[:rawLength]
}

func requireBytes(expr Expr) (error, string) {
	base, _ := unwrap(expr)
	value, ok := base.(*bytesExpr)
	if !ok {
		return expressionError("expected Bytes, got %s", expr.String()), ""
	}
	return nil, value.value
}
