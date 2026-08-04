package lisp

type environment struct {
	parent *environment
	values map[string]*Expr
}

type evaluator struct{}

func Eval(expression Expr) (error, Expr) {
	return (&evaluator{}).eval(expression, prelude())
}

func Run(source string) (error, Expr) {
	err, expression := Read(source)
	if err != nil {
		return err, Expr{}
	}
	return Eval(expression)
}

func (evaluator *evaluator) eval(expression Expr, env *environment) (err error, result Expr) {
	help := expression.help
	defer func() {
		if err == nil && help != "" {
			result.help = help
		}
	}()
	for {
		switch expression.kind {
		case exprBoolean, exprInteger, exprString, exprBytes, exprError, exprNull, exprClosure, exprBuiltin, exprModuleReference:
			return nil, expression
		case exprSymbol:
			return env.lookup(expression.text, expression.span)
		case exprPair:
			err, forms := expressions(expression)
			if err != nil {
				return err, Expr{}
			}
			if len(forms) == 0 {
				return expressionError(expression.span, "cannot evaluate an empty call"), Expr{}
			}
			if isSymbol(forms[0], "quote") {
				return evaluateQuote(forms, expression.span)
			}
			if isSymbol(forms[0], "if") {
				err, next := evaluator.evaluateIf(forms, expression.span, env)
				if err != nil {
					return err, Expr{}
				}
				expression = next
				continue
			}
			if isSymbol(forms[0], "and") {
				return evaluator.evaluateAnd(forms, expression.span, env)
			}
			if isSymbol(forms[0], "or") {
				return evaluator.evaluateOr(forms, expression.span, env)
			}
			if isSymbol(forms[0], "let") {
				err, body, nextEnv := evaluator.evaluateLet(forms, expression.span, env)
				if err != nil {
					return err, Expr{}
				}
				expression, env = body, nextEnv
				continue
			}
			if isSymbol(forms[0], "fn") {
				return evaluator.evaluateFunction(forms, expression.span, env)
			}
			if isSymbol(forms[0], "begin") {
				err, next := evaluator.evaluateBegin(forms, expression.span, env)
				if err != nil {
					return err, Expr{}
				}
				expression = next
				continue
			}

			err, callee := evaluator.eval(forms[0], env)
			if err != nil {
				return err, Expr{}
			}
			if callee.kind == exprBuiltin && callee.builtin.special {
				return callee.builtin.call(evaluator, env, forms[1:], expression.span)
			}
			arguments := make([]Expr, 0, len(forms)-1)
			for _, form := range forms[1:] {
				err, argument := evaluator.eval(form, env)
				if err != nil {
					return err, Expr{}
				}
				arguments = append(arguments, argument)
			}
			if callee.kind == exprClosure {
				err, callEnv := bindClosure(callee.closure, arguments, expression.span)
				if err != nil {
					return err, Expr{}
				}
				expression, env = callee.closure.body, callEnv
				continue
			}
			if callee.kind == exprBuiltin {
				return evaluator.callBuiltin(callee.builtin, env, arguments, expression.span)
			}
			return expressionError(forms[0].span, "%s is not callable", callee.String()), Expr{}
		default:
			return expressionError(expression.span, "invalid expression"), Expr{}
		}
	}
}

func (evaluator *evaluator) call(callee Expr, env *environment, arguments []Expr, span Span) (error, Expr) {
	switch callee.kind {
	case exprClosure:
		err, env := bindClosure(callee.closure, arguments, span)
		if err != nil {
			return err, Expr{}
		}
		return evaluator.eval(callee.closure.body, env)
	case exprBuiltin:
		if callee.builtin.special {
			return expressionError(span, "%s requires direct special-form invocation", callee.String()), Expr{}
		}
		return evaluator.callBuiltin(callee.builtin, env, arguments, span)
	default:
		return expressionError(span, "%s is not callable", callee.String()), Expr{}
	}
}

func (evaluator *evaluator) callBuiltin(builtin *builtin, env *environment, arguments []Expr, span Span) (error, Expr) {
	err, result := builtin.call(evaluator, env, arguments, span)
	if err != nil {
		return err, Expr{}
	}
	if builtin.leaky {
		for _, argument := range arguments {
			if argument.secret {
				result.secret = true
				break
			}
		}
	}
	return nil, result
}

func (evaluator *evaluator) evaluateIf(forms []Expr, span Span, env *environment) (error, Expr) {
	if len(forms) != 4 {
		return expressionError(span, "if requires a condition, then expression, and else expression"), Expr{}
	}
	err, condition := evaluator.eval(forms[1], env)
	if err != nil {
		return err, Expr{}
	}
	err, truth := requireBoolean(condition, forms[1].span)
	if err != nil {
		return err, Expr{}
	}
	if !truth {
		return nil, forms[3]
	}
	return nil, forms[2]
}

func (evaluator *evaluator) evaluateAnd(forms []Expr, _ Span, env *environment) (error, Expr) {
	for _, form := range forms[1:] {
		err, value := evaluator.eval(form, env)
		if err != nil {
			return err, Expr{}
		}
		err, truth := requireBoolean(value, form.span)
		if err != nil {
			return err, Expr{}
		}
		if !truth {
			return nil, boolean(false, Span{})
		}
	}
	return nil, boolean(true, Span{})
}

func (evaluator *evaluator) evaluateOr(forms []Expr, _ Span, env *environment) (error, Expr) {
	for _, form := range forms[1:] {
		err, value := evaluator.eval(form, env)
		if err != nil {
			return err, Expr{}
		}
		err, truth := requireBoolean(value, form.span)
		if err != nil {
			return err, Expr{}
		}
		if truth {
			return nil, boolean(true, Span{})
		}
	}
	return nil, boolean(false, Span{})
}

func (evaluator *evaluator) evaluateLet(forms []Expr, span Span, env *environment) (error, Expr, *environment) {
	if len(forms) != 3 {
		return expressionError(span, "let requires bindings and one body expression"), Expr{}, nil
	}
	err, bindings := expressions(forms[1])
	if err != nil {
		return err, Expr{}, nil
	}
	nextEnv := &environment{parent: env, values: make(map[string]*Expr, len(bindings))}
	type binding struct {
		name string
		init Expr
		span Span
	}
	parsed := make([]binding, 0, len(bindings))
	for _, bindingForm := range bindings {
		err, parts := expressions(bindingForm)
		if err != nil {
			return err, Expr{}, nil
		}
		if len(parts) != 2 || parts[0].kind != exprSymbol {
			return expressionError(bindingForm.span, "let bindings must have a name and initializer"), Expr{}, nil
		}
		if _, exists := nextEnv.values[parts[0].text]; exists {
			return expressionError(parts[0].span, "let binding %q is duplicated", parts[0].text), Expr{}, nil
		}
		nextEnv.values[parts[0].text] = nil
		parsed = append(parsed, binding{name: parts[0].text, init: parts[1], span: parts[0].span})
	}
	for _, binding := range parsed {
		err, value := evaluator.eval(binding.init, nextEnv)
		if err != nil {
			return err, Expr{}, nil
		}
		bound := value
		nextEnv.values[binding.name] = &bound
	}
	return nil, forms[2], nextEnv
}

func (evaluator *evaluator) evaluateFunction(forms []Expr, span Span, env *environment) (error, Expr) {
	if len(forms) != 3 {
		return expressionError(span, "fn requires parameters and one body expression"), Expr{}
	}
	err, parameters := expressions(forms[1])
	if err != nil {
		return err, Expr{}
	}
	names := make([]string, 0, len(parameters))
	seen := make(map[string]struct{}, len(parameters))
	for _, parameter := range parameters {
		if parameter.kind != exprSymbol {
			return expressionError(parameter.span, "fn parameters must be symbols"), Expr{}
		}
		if _, exists := seen[parameter.text]; exists {
			return expressionError(parameter.span, "fn parameter %q is duplicated", parameter.text), Expr{}
		}
		seen[parameter.text] = struct{}{}
		names = append(names, parameter.text)
	}
	return nil, Expr{kind: exprClosure, span: span, closure: &closure{parameters: names, body: forms[2], env: env}}
}

func (evaluator *evaluator) evaluateBegin(forms []Expr, span Span, env *environment) (error, Expr) {
	if len(forms) < 2 {
		return expressionError(span, "begin requires at least one expression"), Expr{}
	}
	for _, form := range forms[1 : len(forms)-1] {
		err, _ := evaluator.eval(form, env)
		if err != nil {
			return err, Expr{}
		}
	}
	return nil, forms[len(forms)-1]
}

func evaluateQuote(forms []Expr, span Span) (error, Expr) {
	if len(forms) != 2 {
		return expressionError(span, "quote requires one expression"), Expr{}
	}
	return nil, forms[1]
}

func bindClosure(closure *closure, arguments []Expr, span Span) (error, *environment) {
	if len(arguments) != len(closure.parameters) {
		return expressionError(span, "function requires %d arguments, got %d", len(closure.parameters), len(arguments)), nil
	}
	env := &environment{parent: closure.env, values: make(map[string]*Expr, len(arguments))}
	for index, parameter := range closure.parameters {
		argument := arguments[index]
		env.values[parameter] = &argument
	}
	return nil, env
}

func (env *environment) lookup(name string, span Span) (error, Expr) {
	for current := env; current != nil; current = current.parent {
		if value, exists := current.values[name]; exists {
			if value == nil {
				return expressionError(span, "binding %q is uninitialized", name), Expr{}
			}
			return nil, *value
		}
	}
	return expressionError(span, "unknown binding %q", name), Expr{}
}

func requireBoolean(expr Expr, span Span) (error, bool) {
	if expr.kind != exprBoolean {
		return expressionError(span, "expected a Boolean, got %s", expr.String()), false
	}
	return nil, expr.boolean
}
