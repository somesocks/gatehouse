package lisp

import (
	"sort"
	"strings"
)

func helpValue(_ *evaluator, env *environment, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError(span, "help requires one argument"), Expr{}
	}
	return nil, stringValue(helpText(arguments[0], env, make(map[string]struct{})), Span{})
}

func helpSearch(_ *evaluator, env *environment, arguments []Expr, span Span) (error, Expr) {
	err, terms := helpTerms(arguments, span)
	if err != nil {
		return err, Expr{}
	}

	bindings := env.matchingBindings(func(_ string, value Expr) bool {
		return value.help != "" && matchesTerms(value.help, terms)
	})
	result := make([]Expr, len(bindings))
	for index, binding := range bindings {
		result[index] = symbol(binding, Span{})
	}
	return nil, list(result, Span{})
}

func helpEnv(_ *evaluator, env *environment, arguments []Expr, span Span) (error, Expr) {
	err, terms := helpTerms(arguments, span)
	if err != nil {
		return err, Expr{}
	}

	bindings := env.matchingBindings(func(name string, _ Expr) bool {
		return matchesTerms(name, terms)
	})
	result := make([]Expr, len(bindings))
	for index, binding := range bindings {
		result[index] = symbol(binding, Span{})
	}
	return nil, list(result, Span{})
}

func helpDocument(_ *evaluator, arguments []Expr, span Span) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError(span, "help/document requires a value and help text"), Expr{}
	}
	if arguments[1].secret {
		return expressionError(span, "help/document requires public help text"), Expr{}
	}
	err, text := requireString(arguments[1], span)
	if err != nil {
		return err, Expr{}
	}
	result := arguments[0]
	result.help = text
	return nil, result
}

func helpTerms(arguments []Expr, span Span) (error, []string) {
	terms := make([]string, len(arguments))
	for index, argument := range arguments {
		err, term := requireString(argument, span)
		if err != nil {
			return err, nil
		}
		terms[index] = strings.ToLower(term)
	}
	return nil, terms
}

func (env *environment) matchingBindings(matches func(string, Expr) bool) []string {
	seen := make(map[string]struct{})
	var bindings []string
	for current := env; current != nil; current = current.parent {
		for name, value := range current.values {
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			if value == nil || !matches(name, *value) {
				continue
			}
			bindings = append(bindings, name)
		}
	}
	sort.Strings(bindings)
	return bindings
}

func matchesTerms(haystack string, terms []string) bool {
	haystack = strings.ToLower(haystack)
	for _, term := range terms {
		if !strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}

func helpText(expr Expr, env *environment, seenSymbols map[string]struct{}) string {
	if expr.kind == exprSymbol {
		if _, exists := seenSymbols[expr.text]; exists {
			return "symbol"
		}
		seenSymbols[expr.text] = struct{}{}
		if value, exists := env.visibleValue(expr.text); exists {
			return helpText(value, env, seenSymbols)
		}
		return "symbol"
	}
	if expr.help != "" {
		return expr.help
	}

	switch expr.kind {
	case exprBoolean:
		return "bool"
	case exprInteger:
		return "int"
	case exprString:
		return "string"
	case exprBytes:
		return "bytes"
	case exprNull:
		return "null"
	case exprPair:
		if isProperList(expr) {
			return "list"
		}
		return "pair"
	case exprClosure:
		return "function (" + strings.Join(expr.closure.parameters, " ") + ")"
	case exprBuiltin:
		return "builtin"
	case exprModuleReference:
		return "module reference"
	default:
		return "invalid"
	}
}

func (env *environment) visibleValue(name string) (Expr, bool) {
	for current := env; current != nil; current = current.parent {
		value, exists := current.values[name]
		if !exists {
			continue
		}
		if value == nil {
			return Expr{}, false
		}
		return *value, true
	}
	return Expr{}, false
}
