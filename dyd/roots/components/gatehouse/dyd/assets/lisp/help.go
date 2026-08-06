package lisp

import (
	"sort"
	"strings"
)

func helpValue(_ *evaluator, env *environment, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("help requires one argument"), nil
	}
	return nil, stringValue(helpText(arguments[0], env, make(map[string]struct{})))
}
func helpSearch(_ *evaluator, env *environment, arguments []Expr) (error, Expr) {
	err, terms := helpTerms(arguments)
	if err != nil {
		return err, nil
	}
	bindings := env.matchingBindings(func(_ string, value Expr) bool { return helpOf(value) != "" && matchesTerms(helpOf(value), terms) })
	for _, form := range evaluatorForms {
		if _, exists := env.visibleValue(form.name); !exists && matchesTerms(form.documentation.text(), terms) {
			bindings = append(bindings, form.name)
		}
	}
	sort.Strings(bindings)
	result := make([]Expr, len(bindings))
	for index, binding := range bindings {
		result[index] = symbol(binding)
	}
	return nil, list(result)
}
func helpEnv(_ *evaluator, env *environment, arguments []Expr) (error, Expr) {
	err, terms := helpTerms(arguments)
	if err != nil {
		return err, nil
	}
	bindings := env.matchingBindings(func(name string, _ Expr) bool { return matchesTerms(name, terms) })
	result := make([]Expr, len(bindings))
	for index, binding := range bindings {
		result[index] = symbol(binding)
	}
	return nil, list(result)
}
func helpDocument(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError("help/document requires a value and help text"), nil
	}
	if hasSecret(arguments[1]) {
		return expressionError("help/document requires public help text"), nil
	}
	err, text := requireString(arguments[1])
	if err != nil {
		return err, nil
	}
	return nil, withHelp(arguments[0], text)
}
func helpTerms(arguments []Expr) (error, []string) {
	terms := make([]string, len(arguments))
	for index, argument := range arguments {
		err, term := requireString(argument)
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
			if value != nil && matches(name, *value) {
				bindings = append(bindings, name)
			}
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
	base, _ := unwrap(expr)
	if symbol, ok := base.(*symbolExpr); ok {
		if _, exists := seenSymbols[symbol.value]; exists {
			return "symbol"
		}
		seenSymbols[symbol.value] = struct{}{}
		if value, exists := env.visibleValue(symbol.value); exists {
			return helpText(value, env, seenSymbols)
		}
		if documentation, exists := evaluatorFormDocumentation(symbol.value); exists {
			return documentation.text()
		}
		return "symbol"
	}
	if help := helpOf(expr); help != "" {
		return help
	}
	switch value := base.(type) {
	case *booleanExpr:
		return "bool"
	case *integerExpr:
		return "int"
	case *stringExpr:
		return "string"
	case *bytesExpr:
		return "bytes"
	case *errorValue:
		return "error"
	case *nullExpr:
		return "null"
	case *pair:
		if isProperList(value) {
			return "list"
		}
		return "pair"
	case *closure:
		return "function (" + strings.Join(value.parameters, " ") + ")"
	case *builtin:
		return "builtin"
	case *moduleReference:
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
			return nil, false
		}
		return *value, true
	}
	return nil, false
}
