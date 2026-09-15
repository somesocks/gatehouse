package lisp

import "context"

type environment struct {
	parent *environment
	values map[string]*Expr
}

type evaluator struct{ context context.Context }

func newEvaluator(ctx context.Context) *evaluator {
	if ctx == nil {
		ctx = context.Background()
	}
	return &evaluator{context: ctx}
}

func Eval(expression Expr) (error, Expr) { return newEvaluator(nil).eval(expression, prelude()) }
func Run(source string) (error, Expr) {
	err, expression := Read(source)
	if err != nil {
		return err, nil
	}
	return Eval(expression)
}

func (evaluator *evaluator) eval(expression Expr, env *environment) (err error, result Expr) {
	_, inputAnnotations := unwrap(expression)
	tailTaint := TaintNone
	defer func() {
		if err == nil {
			result = withTaint(result, joinTaint(inputAnnotations.taint, tailTaint))
		}
		if err == nil && inputAnnotations.help != "" {
			result = withHelp(result, inputAnnotations.help)
		}
	}()
	for {
		if err := evaluator.interrupted(); err != nil {
			return err, nil
		}
		base, _ := unwrap(expression)
		switch value := base.(type) {
		case *booleanExpr, *integerExpr, *stringExpr, *bytesExpr, *errorValue, *nullExpr, *closure, *builtin, *moduleReference:
			return nil, value.(Expr)
		case *symbolExpr:
			return env.lookup(value.value)
		case *pair:
			formsErr, forms := expressions(value)
			if formsErr != nil {
				return formsErr, nil
			}
			if len(forms) == 0 {
				return expressionError("cannot evaluate an empty call"), nil
			}
			err, callee := evaluator.eval(forms[0], env)
			if err != nil {
				return err, nil
			}
			calleeBase, _ := unwrap(callee)
			if builtin, ok := calleeBase.(*builtin); ok {
				arguments := forms[1:]
				if !builtin.special {
					arguments = make([]Expr, 0, len(forms)-1)
					for _, form := range forms[1:] {
						err, argument := evaluator.eval(form, env)
						if err != nil {
							return err, nil
						}
						arguments = append(arguments, argument)
					}
				}
				err, outcome, leaky := evaluator.callBuiltin(builtin, env, arguments)
				if err != nil {
					return err, nil
				}
				switch outcome := outcome.(type) {
				case callResult:
					return nil, withTaint(outcome.value, leaky)
				case callTailState:
					tailTaint = joinTaint(tailTaint, joinTaint(leaky, outcome.taint))
					expression = outcome.expression
					if outcome.environment != nil {
						env = outcome.environment
					}
					continue
				default:
					panic("invalid builtin call outcome")
				}
			}
			arguments := make([]Expr, 0, len(forms)-1)
			for _, form := range forms[1:] {
				err, argument := evaluator.eval(form, env)
				if err != nil {
					return err, nil
				}
				arguments = append(arguments, argument)
			}
			if closure, ok := calleeBase.(*closure); ok {
				err, callEnv := bindClosure(closure, arguments)
				if err != nil {
					return err, nil
				}
				for _, form := range closure.body[:len(closure.body)-1] {
					err, _ = evaluator.eval(form, callEnv)
					if err != nil {
						return err, nil
					}
				}
				expression, env = closure.body[len(closure.body)-1], callEnv
				continue
			}
			return expressionError("%s is not callable", callee.String()), nil
		default:
			return expressionError("invalid expression"), nil
		}
	}
}

func (evaluator *evaluator) call(callee Expr, env *environment, arguments []Expr) (error, Expr) {
	if err := evaluator.interrupted(); err != nil {
		return err, nil
	}
	base, _ := unwrap(callee)
	switch callee := base.(type) {
	case *closure:
		err, env := bindClosure(callee, arguments)
		if err != nil {
			return err, nil
		}
		for _, form := range callee.body[:len(callee.body)-1] {
			err, _ = evaluator.eval(form, env)
			if err != nil {
				return err, nil
			}
		}
		return evaluator.eval(callee.body[len(callee.body)-1], env)
	case *builtin:
		err, outcome, leaky := evaluator.callBuiltin(callee, env, arguments)
		if err != nil {
			return err, nil
		}
		err, result := evaluator.resolveCallOutcome(outcome, env)
		if err != nil {
			return err, nil
		}
		return nil, withTaint(result, leaky)
	default:
		return expressionError("%s is not callable", callee.String()), nil
	}
}

func (evaluator *evaluator) callBuiltin(builtin *builtin, env *environment, arguments []Expr) (error, callOutcome, Taint) {
	if err := evaluator.interrupted(); err != nil {
		return err, nil, TaintNone
	}
	taint := TaintNone
	if builtin.leaky {
		for _, argument := range arguments {
			taint = joinTaint(taint, TaintOf(argument))
		}
	}
	err, outcome := builtin.call(evaluator, env, arguments)
	if err != nil {
		return taintError(err, taint), nil, TaintNone
	}
	if err := evaluator.interrupted(); err != nil {
		return err, nil, TaintNone
	}
	return nil, outcome, taint
}

func (evaluator *evaluator) interrupted() error {
	if evaluator.context == nil || evaluator.context.Err() == nil {
		return nil
	}
	return interruptedError{cause: evaluator.context.Err()}
}

func (evaluator *evaluator) resolveCallOutcome(outcome callOutcome, env *environment) (error, Expr) {
	switch outcome := outcome.(type) {
	case callResult:
		return nil, outcome.value
	case callTailState:
		if outcome.environment == nil {
			outcome.environment = env
		}
		err, result := evaluator.eval(outcome.expression, outcome.environment)
		if err != nil {
			return err, nil
		}
		return nil, withTaint(result, outcome.taint)
	default:
		panic("invalid builtin call outcome")
	}
}

func (evaluator *evaluator) evaluateIf(forms []Expr, env *environment) (error, Expr, Taint) {
	if len(forms) != 3 {
		return expressionError("if requires a condition, then expression, and else expression"), nil, TaintNone
	}
	err, condition := evaluator.eval(forms[0], env)
	if err != nil {
		return err, nil, TaintNone
	}
	err, truth := requireBoolean(condition)
	if err != nil {
		return err, nil, TaintNone
	}
	if truth {
		return nil, forms[1], TaintOf(condition)
	}
	return nil, forms[2], TaintOf(condition)
}
func (evaluator *evaluator) evaluateAnd(forms []Expr, env *environment) (error, Expr) {
	taint := TaintNone
	for _, form := range forms {
		err, value := evaluator.eval(form, env)
		if err != nil {
			return err, nil
		}
		taint = joinTaint(taint, TaintOf(value))
		err, truth := requireBoolean(value)
		if err != nil {
			return err, nil
		}
		if !truth {
			return nil, withTaint(boolean(false), taint)
		}
	}
	return nil, withTaint(boolean(true), taint)
}
func (evaluator *evaluator) evaluateOr(forms []Expr, env *environment) (error, Expr) {
	taint := TaintNone
	for _, form := range forms {
		err, value := evaluator.eval(form, env)
		if err != nil {
			return err, nil
		}
		taint = joinTaint(taint, TaintOf(value))
		err, truth := requireBoolean(value)
		if err != nil {
			return err, nil
		}
		if truth {
			return nil, withTaint(boolean(true), taint)
		}
	}
	return nil, withTaint(boolean(false), taint)
}

func (evaluator *evaluator) evaluateLet(forms []Expr, env *environment) (error, Expr, *environment) {
	if len(forms) < 2 {
		return expressionError("let requires bindings and at least one body expression"), nil, nil
	}
	err, bindings := expressions(forms[0])
	if err != nil {
		return err, nil, nil
	}
	nextEnv := &environment{parent: env, values: make(map[string]*Expr, len(bindings))}
	type binding struct {
		name string
		init Expr
	}
	parsed := make([]binding, 0, len(bindings))
	for _, bindingForm := range bindings {
		err, parts := expressions(bindingForm)
		if err != nil {
			return err, nil, nil
		}
		if len(parts) != 2 {
			return expressionError("let bindings must have a name and initializer"), nil, nil
		}
		base, _ := unwrap(parts[0])
		name, ok := base.(*symbolExpr)
		if !ok {
			return expressionError("let bindings must have a name and initializer"), nil, nil
		}
		if _, exists := nextEnv.values[name.value]; exists {
			return expressionError("let binding %q is duplicated", name.value), nil, nil
		}
		nextEnv.values[name.value] = nil
		parsed = append(parsed, binding{name: name.value, init: parts[1]})
	}
	for _, binding := range parsed {
		err, value := evaluator.eval(binding.init, nextEnv)
		if err != nil {
			return err, nil, nil
		}
		bound := value
		nextEnv.values[binding.name] = &bound
	}
	for _, form := range forms[1 : len(forms)-1] {
		err, _ = evaluator.eval(form, nextEnv)
		if err != nil {
			return err, nil, nil
		}
	}
	return nil, forms[len(forms)-1], nextEnv
}
func (evaluator *evaluator) evaluateFunction(forms []Expr, env *environment) (error, Expr) {
	if len(forms) < 2 {
		return expressionError("fn requires parameters and at least one body expression"), nil
	}
	err, names, rest := functionParameters(forms[0])
	if err != nil {
		return err, nil
	}
	return nil, &closure{parameters: names, restParameter: rest, body: forms[1:], env: env}
}

func functionParameters(expression Expr) (error, []string, *string) {
	names := []string{}
	seen := map[string]struct{}{}
	for {
		base, _ := unwrap(expression)
		switch value := base.(type) {
		case *nullExpr:
			return nil, names, nil
		case *symbolExpr:
			if _, exists := seen[value.value]; exists {
				return expressionError("fn parameter %q is duplicated", value.value), nil, nil
			}
			rest := value.value
			return nil, names, &rest
		case *pair:
			parameter, _ := unwrap(value.first)
			symbol, ok := parameter.(*symbolExpr)
			if !ok {
				return expressionError("fn parameters must be symbols"), nil, nil
			}
			if _, exists := seen[symbol.value]; exists {
				return expressionError("fn parameter %q is duplicated", symbol.value), nil, nil
			}
			seen[symbol.value] = struct{}{}
			names = append(names, symbol.value)
			expression = value.rest
		default:
			return expressionError("fn parameters must be symbols"), nil, nil
		}
	}
}
func (evaluator *evaluator) evaluateBegin(forms []Expr, env *environment) (error, Expr) {
	if len(forms) == 0 {
		return expressionError("begin requires at least one expression"), nil
	}
	for _, form := range forms[:len(forms)-1] {
		err, _ := evaluator.eval(form, env)
		if err != nil {
			return err, nil
		}
	}
	return nil, forms[len(forms)-1]
}
func bindClosure(closure *closure, arguments []Expr) (error, *environment) {
	if closure.restParameter == nil && len(arguments) != len(closure.parameters) {
		return expressionError("function requires %d arguments, got %d", len(closure.parameters), len(arguments)), nil
	}
	if closure.restParameter != nil && len(arguments) < len(closure.parameters) {
		return expressionError("function requires at least %d arguments, got %d", len(closure.parameters), len(arguments)), nil
	}
	env := &environment{parent: closure.env, values: make(map[string]*Expr, len(closure.parameters)+1)}
	for index, parameter := range closure.parameters {
		argument := arguments[index]
		env.values[parameter] = &argument
	}
	if closure.restParameter != nil {
		rest := list(arguments[len(closure.parameters):])
		env.values[*closure.restParameter] = &rest
	}
	return nil, env
}
func (env *environment) lookup(name string) (error, Expr) {
	for current := env; current != nil; current = current.parent {
		if value, exists := current.values[name]; exists {
			if value == nil {
				return expressionError("binding %q is uninitialized", name), nil
			}
			return nil, *value
		}
	}
	return expressionError("unknown binding %q", name), nil
}
func requireBoolean(expr Expr) (error, bool) {
	base, _ := unwrap(expr)
	value, ok := base.(*booleanExpr)
	if !ok {
		return expressionError("expected a Boolean, got %s", expr.String()), false
	}
	return nil, value.value
}
