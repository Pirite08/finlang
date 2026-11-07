package eval

import (
	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

func ruleBuiltin(name string) bool {
	spec, known := builtins[name]
	return known && spec.scope != builtinOutput
}

func reservedRuleName(name string) bool {
	spec, known := builtins[name]
	return known && spec.scope != builtinBasic
}

func rulePolicyError(span diagnostic.Span, message string) error {
	return diagnostic.WithSpan(diagnostic.New("E_RULE_POLICY", message), span)
}

func validateRulePolicy(rule *ast.Rule, maxDepth int) error {
	if rule == nil {
		return diagnostic.New("E_RULE_POLICY", "rule-only mode requires a rule declaration")
	}
	for _, declaration := range rule.Inputs {
		if reservedRuleName(declaration.Name) {
			return rulePolicyError(declaration.Span, "builtin names are reserved in rule-only mode")
		}
	}
	if reservedRuleName(rule.Output.Name) {
		return rulePolicyError(rule.Output.Span, "builtin names are reserved in rule-only mode")
	}
	if len(rule.Steps) == 0 || rule.Return == nil {
		return rulePolicyError(rule.Span, "a rule must have steps and a final return")
	}
	output, ok := rule.Return.Value.(*ast.Identifier)
	if !ok || output == nil || output.Name != rule.Output.Name {
		return rulePolicyError(rule.Return.Span, "the final return must name the declared output")
	}
	if maxDepth <= 0 {
		maxDepth = config.DefaultLimits().MaxSyntaxDepth
	}
	for _, step := range rule.Steps {
		if step == nil || step.Body == nil {
			return rulePolicyError(rule.Span, "a step must have a body")
		}
		if err := validateRuleNode(step.Body, maxDepth-2); err != nil {
			return err
		}
	}
	return nil
}

// Traverse the entire tree before any evaluation. Only the explicit node and
// operator cases below are admitted, even in branches that would not execute.
func validateRuleNode(node ast.Node, depth int) error {
	if node == nil || depth < 0 {
		return diagnostic.New("E_RULE_POLICY", "invalid or excessively deep rule syntax")
	}
	childDepth := depth - 1
	switch node := node.(type) {
	case *ast.Block:
		for _, statement := range node.Statements {
			if err := validateRuleNode(statement, childDepth); err != nil {
				return err
			}
		}
		return nil
	case *ast.Let:
		if reservedRuleName(node.Name) {
			return rulePolicyError(node.Span, "builtin names are reserved in rule-only mode")
		}
		return validateRuleNode(node.Value, childDepth)
	case *ast.Require:
		return validateRuleNode(node.Condition, childDepth)
	case *ast.If:
		if err := validateRuleNode(node.Condition, childDepth); err != nil {
			return err
		}
		if node.Consequence == nil {
			return rulePolicyError(node.Span, "a conditional must have a body")
		}
		if err := validateRuleNode(node.Consequence, childDepth); err != nil {
			return err
		}
		if node.Alternative != nil {
			return validateRuleNode(node.Alternative, childDepth)
		}
		return nil
	case *ast.Number, *ast.String, *ast.Bool:
		return nil
	case *ast.Identifier:
		if reservedRuleName(node.Name) {
			return rulePolicyError(node.Span, "builtins may only appear as direct calls in rule-only mode")
		}
		return nil
	case *ast.Prefix:
		if node.Operator == "-" || node.Operator == "!" {
			return validateRuleNode(node.Right, childDepth)
		}
	case *ast.Infix:
		switch node.Operator {
		case "+", "-", "*", "==", "!=", "<", "<=", ">", ">=":
			if err := validateRuleNode(node.Left, childDepth); err != nil {
				return err
			}
			return validateRuleNode(node.Right, childDepth)
		}
	case *ast.Call:
		name, ok := node.Function.(*ast.Identifier)
		if !ok || name == nil || !ruleBuiltin(name.Name) {
			return rulePolicyError(node.Span, "only direct calls to approved calculation functions are allowed")
		}
		for _, arg := range node.Arguments {
			if err := validateRuleNode(arg, childDepth); err != nil {
				return err
			}
		}
		return nil
	}
	return rulePolicyError(node.GetSpan(), "syntax is unavailable in rule-only mode")
}
