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
	terms := make([]string, len(arguments))
	for index, argument := range arguments {
		err, term := requireString(argument, span)
		if err != nil {
			return err, Expr{}
		}
		terms[index] = strings.ToLower(term)
	}

	bindings := env.documentedBindings(terms)
	result := make([]Expr, len(bindings))
	for index, binding := range bindings {
		result[index] = symbol(binding.name, Span{})
	}
	return nil, list(result, Span{})
}

type documentedBinding struct {
	name string
}

func (env *environment) documentedBindings(terms []string) []documentedBinding {
	seen := make(map[string]struct{})
	var bindings []documentedBinding
	for current := env; current != nil; current = current.parent {
		for name, value := range current.values {
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			if value == nil || !matchesHelp(*value, name, terms) {
				continue
			}
			bindings = append(bindings, documentedBinding{name: name})
		}
	}
	sort.Slice(bindings, func(left int, right int) bool {
		return bindings[left].name < bindings[right].name
	})
	return bindings
}

func matchesHelp(expr Expr, name string, terms []string) bool {
	if expr.kind != exprBuiltin {
		return false
	}
	haystack := strings.ToLower(name + "\n" + expr.builtin.helpText + "\n" + strings.Join(expr.builtin.helpKeywords, "\n"))
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

	switch expr.kind {
	case exprBoolean:
		return "bool"
	case exprInteger:
		return "int"
	case exprString:
		return "string"
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
		return expr.builtin.helpText
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
