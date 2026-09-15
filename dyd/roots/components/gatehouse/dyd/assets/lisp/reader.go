package lisp

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

func Read(source string) (error, Expr) {
	reader := reader{source: source}
	err, comments := reader.readTrivia()
	if err != nil {
		return err, nil
	}
	if reader.atEnd() {
		if len(comments) != 0 {
			return reader.unattachedComment(comments[0])
		}
		return reader.parseError("expected_expression", reader.position, nil, "expected an expression"), nil
	}
	err, expr := reader.readExprWithLeadingComments(comments)
	if err != nil {
		return err, nil
	}
	err, comments = reader.readTrivia()
	if err != nil {
		return err, nil
	}
	for _, comment := range comments {
		if comment.leading {
			return reader.unattachedComment(comment)
		}
		expr = appendHelp(expr, comment.text)
	}
	if !reader.atEnd() {
		return reader.parseError("expected_end_of_input", reader.position, nil, "expected end of input"), nil
	}
	return nil, expr
}

type reader struct {
	source   string
	position int
}

type commentBlock struct {
	text     string
	leading  bool
	position int
}

func (reader *reader) readExpr() (error, Expr) {
	if reader.atEnd() {
		return reader.parseError("expected_expression", reader.position, nil, "expected an expression"), nil
	}

	switch reader.source[reader.position] {
	case '(':
		return reader.readList()
	case ')':
		position := reader.position
		reader.position++
		return reader.parseError("unexpected_closing_parenthesis", position, nil, "unexpected closing parenthesis"), nil
	case '\'':
		reader.position++
		err, comments := reader.readTrivia()
		if err != nil {
			return err, nil
		}
		err, quoted := reader.readExprWithLeadingComments(comments)
		if err != nil {
			return err, nil
		}
		return nil, list([]Expr{symbol("quote"), quoted})
	case '"':
		return reader.readString()
	case '@':
		return reader.readModuleReference()
	default:
		return reader.readAtom()
	}
}

func (reader *reader) readModuleReference() (error, Expr) {
	position := reader.position
	reader.position++
	start := reader.position
	for !reader.atEnd() && !isDelimiter(reader.source[reader.position]) {
		_, size := utf8.DecodeRuneInString(reader.source[reader.position:])
		reader.position += size
	}
	err, reference := moduleReferenceValue(reader.source[start:reader.position])
	if err != nil {
		return reader.parseError("invalid_module_reference", position, nil, err.Error()), nil
	}
	return nil, reference
}

func (reader *reader) readList() (error, Expr) {
	opening := reader.position
	reader.position++
	var values []Expr
	for {
		err, comments := reader.readTrivia()
		if err != nil {
			return err, nil
		}
		if reader.atEnd() {
			if len(comments) != 0 {
				return reader.unattachedComment(comments[0])
			}
			return reader.parseError("unterminated_list", reader.position, &opening, "unterminated list"), nil
		}
		if reader.source[reader.position] == ')' {
			for _, comment := range comments {
				if comment.leading || len(values) == 0 {
					return reader.unattachedComment(comment)
				}
				values[len(values)-1] = appendHelp(values[len(values)-1], comment.text)
			}
			reader.position++
			return nil, list(values)
		}
		var leading []commentBlock
		for _, comment := range comments {
			if comment.leading {
				leading = append(leading, comment)
				continue
			}
			if len(values) == 0 {
				return reader.unattachedComment(comment)
			}
			values[len(values)-1] = appendHelp(values[len(values)-1], comment.text)
		}
		position := reader.position
		err, value := reader.readExprWithLeadingComments(leading)
		if err != nil {
			return err, nil
		}
		if isSymbol(value, ".") {
			if len(values) == 0 {
				return reader.parseError("dotted_list_requires_preceding_value", position, nil, "dotted list requires a preceding value"), nil
			}
			err, comments := reader.readTrivia()
			if err != nil {
				return err, nil
			}
			if reader.atEnd() {
				return reader.parseError("dotted_list_requires_tail", reader.position, nil, "dotted list requires a tail"), nil
			}
			if reader.source[reader.position] == ')' {
				return reader.parseError("dotted_list_requires_tail", reader.position, nil, "dotted list requires a tail"), nil
			}
			err, tail := reader.readExprWithLeadingComments(comments)
			if err != nil {
				return err, nil
			}
			err, comments = reader.readTrivia()
			if err != nil {
				return err, nil
			}
			if reader.atEnd() || reader.source[reader.position] != ')' {
				return reader.parseError("dotted_list_tail_requires_closing_parenthesis", reader.position, nil, "dotted list tail must be followed by a closing parenthesis"), nil
			}
			for _, comment := range comments {
				if comment.leading {
					return reader.unattachedComment(comment)
				}
				tail = appendHelp(tail, comment.text)
			}
			reader.position++
			for index := len(values) - 1; index >= 0; index-- {
				tail = pairValue(values[index], tail)
			}
			return nil, tail
		}
		values = append(values, value)
	}
}

func (reader *reader) readString() (error, Expr) {
	opening := reader.position
	reader.position++
	var value []rune
	for !reader.atEnd() {
		r, size := utf8.DecodeRuneInString(reader.source[reader.position:])
		reader.position += size
		if r == '"' {
			return nil, stringValue(string(value))
		}
		if r != '\\' {
			value = append(value, r)
			continue
		}
		if reader.atEnd() {
			break
		}
		escapePosition := reader.position
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
			return reader.parseError("unsupported_escape_sequence", escapePosition, nil, "unsupported escape sequence"), nil
		}
	}
	return reader.parseError("unterminated_string", reader.position, &opening, "unterminated string"), nil
}

func (reader *reader) readAtom() (error, Expr) {
	start := reader.position
	for !reader.atEnd() && !isDelimiter(reader.source[reader.position]) {
		_, size := utf8.DecodeRuneInString(reader.source[reader.position:])
		reader.position += size
	}
	text := reader.source[start:reader.position]
	switch text {
	case "#t":
		return nil, boolean(true)
	case "#f":
		return nil, boolean(false)
	}
	if value, err := strconv.ParseInt(text, 10, 64); err == nil {
		return nil, integer(value)
	}
	return nil, symbol(text)
}

func (reader *reader) readExprWithLeadingComments(comments []commentBlock) (error, Expr) {
	for _, comment := range comments {
		if !comment.leading {
			return reader.unattachedComment(comment)
		}
	}
	err, expression := reader.readExpr()
	if err != nil {
		return err, nil
	}
	for _, comment := range comments {
		expression = appendHelp(expression, comment.text)
	}
	return nil, expression
}

func (reader *reader) readTrivia() (error, []commentBlock) {
	var comments []commentBlock
	var current *commentBlock
	for {
		newlines := reader.skipWhitespace()
		if current != nil && newlines > 1 {
			comments = append(comments, *current)
			current = nil
		}
		if reader.atEnd() || reader.source[reader.position] != ';' {
			break
		}

		leading := reader.isLeadingComment(reader.position)
		reader.position++
		textStart := reader.position
		for !reader.atEnd() && reader.source[reader.position] != '\n' && reader.source[reader.position] != '\r' {
			_, size := utf8.DecodeRuneInString(reader.source[reader.position:])
			reader.position += size
		}
		text := strings.TrimSpace(reader.source[textStart:reader.position])
		if current == nil {
			current = &commentBlock{text: text, leading: leading, position: textStart - 1}
		} else {
			current.text += "\n" + text
		}
	}
	if current != nil {
		comments = append(comments, *current)
	}
	return nil, comments
}

func (reader *reader) skipWhitespace() int {
	newlines := 0
	for !reader.atEnd() {
		switch reader.source[reader.position] {
		case ' ', '\t':
			reader.position++
		case '\n':
			reader.position++
			newlines++
		case '\r':
			reader.position++
			if !reader.atEnd() && reader.source[reader.position] == '\n' {
				reader.position++
			}
			newlines++
		default:
			return newlines
		}
	}
	return newlines
}

func (reader *reader) isLeadingComment(position int) bool {
	for position > 0 {
		position--
		switch reader.source[position] {
		case ' ', '\t':
			continue
		case '\n', '\r':
			return true
		default:
			return false
		}
	}
	return true
}

func (reader *reader) unattachedComment(comment commentBlock) (error, Expr) {
	return reader.parseError("comment_has_no_target", comment.position, nil, "comment has no target"), nil
}

func (reader *reader) parseError(code string, position int, opening *int, message string) error {
	result := &ParseError{Code: code, Message: message, At: sourceLocation(reader.source, position)}
	if opening != nil {
		location := sourceLocation(reader.source, *opening)
		result.Opening = &location
	}
	return result
}

func sourceLocation(source string, position int) SourceLocation {
	if position < 0 {
		position = 0
	}
	if position > len(source) {
		position = len(source)
	}
	result := SourceLocation{Offset: position, Line: 1, Column: 1}
	for index := 0; index < position; {
		switch source[index] {
		case '\n':
			index++
			result.Line++
			result.Column = 1
		case '\r':
			index++
			if index < position && source[index] == '\n' {
				index++
			}
			result.Line++
			result.Column = 1
		default:
			_, size := utf8.DecodeRuneInString(source[index:])
			index += size
			result.Column++
		}
	}
	return result
}

func (reader *reader) atEnd() bool {
	return reader.position >= len(reader.source)
}

func isDelimiter(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r' || value == '(' || value == ')' || value == '\'' || value == '"' || value == ';'
}
