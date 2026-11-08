package eval

import (
	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

func ruleBuiltin(name string, collections bool) bool {
	if collections {
		switch name {
		case "len", "range", "all", "any", "sorted", "lower_bound", "enumerate", "zip_exact":
			return true
		}
	}
	spec, known := builtins[name]
	return known && (spec.scope == builtinStandard || spec.scope == builtinBasic || spec.scope == builtinFX)
}

type rulePolicy struct {
	collections bool
	maxItems    int
}

func (p rulePolicy) reservedName(name string) bool {
	spec, known := builtins[name]
	return known && (spec.scope == builtinStandard || spec.scope == builtinFX || spec.scope == builtinOutput ||
		(p.collections && spec.scope == builtinCollection))
}

func ruleMapMethod(name string) bool {
	switch name {
	case "len", "has", "get", "with", "keys":
		return true
	}
	return false
}

func rulePolicyError(span diagnostic.Span, message string) error {
	return diagnostic.WithSpan(diagnostic.New("E_RULE_POLICY", message), span)
}

func (p rulePolicy) validate(rule *ast.Rule, maxDepth int) error {
	if rule == nil {
		return diagnostic.New("E_RULE_POLICY", "rule-only mode requires a rule declaration")
	}
	for _, declaration := range rule.Inputs {
		if p.reservedName(declaration.Name) {
			return rulePolicyError(declaration.Span, "builtin names are reserved in rule-only mode")
		}
		if !p.collections && declaration.Type == "record" {
			return rulePolicyError(declaration.Span, "Record input requires the collection rule profile")
		}
	}
	if p.reservedName(rule.Output.Name) {
		return rulePolicyError(rule.Output.Span, "builtin names are reserved in rule-only mode")
	}
	if !p.collections && rule.Output.Type == "record" {
		return rulePolicyError(rule.Output.Span, "Record output requires the collection rule profile")
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
		if err := p.node(step.Body, maxDepth-2); err != nil {
			return err
		}
	}
	return nil
}

// Traverse the entire tree before any evaluation. Only the explicit node and
// operator cases below are admitted, even in branches that would not execute.
func (p rulePolicy) node(node ast.Node, depth int) error {
	if node == nil || depth < 0 {
		return diagnostic.New("E_RULE_POLICY", "invalid or excessively deep rule syntax")
	}
	childDepth := depth - 1
	switch node := node.(type) {
	case *ast.Block:
		for _, statement := range node.Statements {
			if err := p.node(statement, childDepth); err != nil {
				return err
			}
		}
		return nil
	case *ast.Let:
		return p.binding(node.Name, node.Span, node.Value, childDepth)
	case *ast.Require:
		return p.node(node.Condition, childDepth)
	case *ast.If:
		if err := p.node(node.Condition, childDepth); err != nil {
			return err
		}
		if node.Consequence == nil {
			return rulePolicyError(node.Span, "a conditional must have a body")
		}
		if err := p.node(node.Consequence, childDepth); err != nil {
			return err
		}
		if node.Alternative != nil {
			return p.node(node.Alternative, childDepth)
		}
		return nil
	case *ast.IfExpr:
		if err := p.node(node.Condition, childDepth); err != nil {
			return err
		}
		if err := p.node(node.Consequence, childDepth); err != nil {
			return err
		}
		return p.node(node.Alternative, childDepth)
	case *ast.Number, *ast.String, *ast.Bool:
		return nil
	case *ast.Identifier:
		if p.reservedName(node.Name) {
			return rulePolicyError(node.Span, "builtins may only appear as direct calls in rule-only mode")
		}
		return nil
	case *ast.Prefix:
		if node.Operator == "-" || node.Operator == "!" {
			return p.node(node.Right, childDepth)
		}
	case *ast.Infix:
		switch node.Operator {
		case "+", "-", "*", "==", "!=", "<", "<=", ">", ">=", "&&", "||":
			if err := p.node(node.Left, childDepth); err != nil {
				return err
			}
			return p.node(node.Right, childDepth)
		}
	case *ast.Call:
		return p.call(node, childDepth)
	}
	if p.collections {
		return p.collectionNode(node, childDepth)
	}
	return rulePolicyError(node.GetSpan(), "syntax is unavailable in rule-only mode")
}

func (p rulePolicy) binding(name string, span diagnostic.Span, value ast.Expr, depth int) error {
	if p.reservedName(name) {
		return rulePolicyError(span, "builtin names are reserved in rule-only mode")
	}
	return p.node(value, depth)
}

func (p rulePolicy) call(call *ast.Call, depth int) error {
	switch callee := call.Function.(type) {
	case *ast.Identifier:
		if callee == nil || !ruleBuiltin(callee.Name, p.collections) {
			return rulePolicyError(call.Span, "only direct calls to approved calculation functions are allowed")
		}
	case *ast.Member:
		if !p.collections || callee == nil {
			return rulePolicyError(call.Span, "method calls are unavailable in this rule profile")
		}
		if !ruleMapMethod(callee.Name) {
			return rulePolicyError(call.Span, "only approved Map methods may be called in a rule")
		}
		if err := p.node(callee, depth); err != nil {
			return err
		}
	default:
		return rulePolicyError(call.Span, "only direct calls to approved calculation functions are allowed")
	}
	for _, arg := range call.Arguments {
		if err := p.node(arg, depth); err != nil {
			return err
		}
	}
	return nil
}

func (p rulePolicy) collectionNode(node ast.Node, depth int) error {
	switch node := node.(type) {
	case *ast.Var:
		return p.binding(node.Name, node.Span, node.Value, depth)
	case *ast.Assign:
		return p.binding(node.Name, node.Span, node.Value, depth)
	case *ast.For:
		if err := p.binding(node.Name, node.Span, node.Iterable, depth); err != nil {
			return err
		}
		return p.node(node.Body, depth)
	case *ast.List:
		if err := checkCollectionSize(int64(len(node.Elements)), p.maxItems, node.Span); err != nil {
			return err
		}
		for _, element := range node.Elements {
			if err := p.node(element, depth); err != nil {
				return err
			}
		}
		return nil
	case *ast.Record:
		return p.fields(node.Fields, node.Span, depth)
	case *ast.Map:
		return p.fields(node.Entries, node.Span, depth)
	case *ast.Member:
		return p.node(node.Receiver, depth)
	case *ast.Index:
		if err := p.node(node.Collection, depth); err != nil {
			return err
		}
		return p.node(node.Index, depth)
	}
	return rulePolicyError(node.GetSpan(), "syntax is unavailable in rule-only mode")
}

func (p rulePolicy) fields(fields []ast.NamedField, span diagnostic.Span, depth int) error {
	if err := checkCollectionSize(int64(len(fields)), p.maxItems, span); err != nil {
		return err
	}
	for _, field := range fields {
		if err := p.node(field.Value, depth); err != nil {
			return err
		}
	}
	return nil
}
