package lisp

import (
	"strconv"
	"unicode/utf8"
)

func Read(source string) (error, Expr) {
	reader := reader{source: source}
	reader.skipIgnored()
	if reader.atEnd() {
		return expressionError(Span{}, "expected an expression"), Expr{}
	}
	err, expr := reader.readExpr()
	if err != nil {
		return err, Expr{}
	}
	reader.skipIgnored()
	if !reader.atEnd() {
		_, size := utf8.DecodeRuneInString(reader.source[reader.position:])
		return expressionError(Span{Start: reader.position, End: reader.position + size}, "expected end of input"), Expr{}
	}
	return nil, expr
}

type reader struct {
	source   string
	position int
}

func (reader *reader) readExpr() (error, Expr) {
	reader.skipIgnored()
	start := reader.position
	if reader.atEnd() {
		return expressionError(Span{Start: start, End: start}, "expected an expression"), Expr{}
	}

	switch reader.source[reader.position] {
	case '(':
		return reader.readList()
	case ')':
		reader.position++
		return expressionError(Span{Start: start, End: reader.position}, "unexpected closing parenthesis"), Expr{}
	case '\'':
		reader.position++
		err, quoted := reader.readExpr()
		if err != nil {
			return err, Expr{}
		}
		return nil, list([]Expr{symbol("quote", Span{Start: start, End: start + 1}), quoted}, Span{Start: start, End: reader.position})
	case '"':
		return reader.readString()
	default:
		return reader.readAtom()
	}
}

func (reader *reader) readList() (error, Expr) {
	start := reader.position
	reader.position++
	var values []Expr
	for {
		reader.skipIgnored()
		if reader.atEnd() {
			return expressionError(Span{Start: start, End: reader.position}, "unterminated list"), Expr{}
		}
		if reader.source[reader.position] == ')' {
			reader.position++
			return nil, list(values, Span{Start: start, End: reader.position})
		}
		err, value := reader.readExpr()
		if err != nil {
			return err, Expr{}
		}
		values = append(values, value)
	}
}

func (reader *reader) readString() (error, Expr) {
	start := reader.position
	reader.position++
	var value []rune
	for !reader.atEnd() {
		r, size := utf8.DecodeRuneInString(reader.source[reader.position:])
		reader.position += size
		if r == '"' {
			return nil, stringValue(string(value), Span{Start: start, End: reader.position})
		}
		if r != '\\' {
			value = append(value, r)
			continue
		}
		if reader.atEnd() {
			break
		}
		escaped, size := utf8.DecodeRuneInString(reader.source[reader.position:])
		reader.position += size
		switch escaped {
		case '"', '\\':
			value = append(value, escaped)
		case 'n':
			value = append(value, '\n')
		case 'r':
			value = append(value, '\r')
		case 't':
			value = append(value, '\t')
		default:
			return expressionError(Span{Start: reader.position - size - 1, End: reader.position}, "unsupported escape sequence"), Expr{}
		}
	}
	return expressionError(Span{Start: start, End: reader.position}, "unterminated string"), Expr{}
}

func (reader *reader) readAtom() (error, Expr) {
	start := reader.position
	for !reader.atEnd() && !isDelimiter(reader.source[reader.position]) {
		_, size := utf8.DecodeRuneInString(reader.source[reader.position:])
		reader.position += size
	}
	text := reader.source[start:reader.position]
	span := Span{Start: start, End: reader.position}
	switch text {
	case "#t":
		return nil, boolean(true, span)
	case "#f":
		return nil, boolean(false, span)
	}
	if value, err := strconv.ParseInt(text, 10, 64); err == nil {
		return nil, integer(value, span)
	}
	return nil, symbol(text, span)
}

func (reader *reader) skipIgnored() {
	for {
		for !reader.atEnd() {
			r, size := utf8.DecodeRuneInString(reader.source[reader.position:])
			if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
				break
			}
			reader.position += size
		}
		if reader.atEnd() || reader.source[reader.position] != ';' {
			return
		}
		for !reader.atEnd() && reader.source[reader.position] != '\n' {
			_, size := utf8.DecodeRuneInString(reader.source[reader.position:])
			reader.position += size
		}
	}
}

func (reader *reader) atEnd() bool {
	return reader.position >= len(reader.source)
}

func isDelimiter(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r' || value == '(' || value == ')' || value == '\'' || value == '"' || value == ';'
}
