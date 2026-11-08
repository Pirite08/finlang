package cli

import (
	"bytes"
	"fmt"
	"os"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/fx"
)

const ruleOnlyProfile = "rule-only-v1"
const ruleCollectionProfile = "rule-only-v2"

func (c command) hasFXSource() bool {
	return c.fxFixed != "" || c.fxSnapshot != "" || c.fxURL != "" || c.fxProvider != ""
}

func (c command) fxMode() string {
	switch {
	case c.replayAudit != "":
		return "replay"
	case c.fxFixed != "":
		return "fixed"
	case c.fxSnapshot != "":
		return "snapshot"
	case c.fxURL != "":
		return "http"
	case c.fxProvider == "alpha-vantage":
		return "alpha-vantage"
	default:
		return "none"
	}
}

func (c command) validateFXFlags(ageSet bool) error {
	count := 0
	for _, source := range []string{c.fxFixed, c.fxSnapshot, c.fxURL, c.fxProvider} {
		if source != "" {
			count++
		}
	}
	if count > 1 {
		return diagnostic.New("E_CLI_USAGE", "choose one of -fx-fixed, -fx-snapshot, -fx-url or -fx-provider")
	}
	if count > 0 && !c.json {
		return diagnostic.New("E_CLI_USAGE", "FX source options require -json rule execution")
	}
	if ageSet && (c.fxPolicy.MaxAge <= 0 || (c.fxURL == "" && c.fxProvider == "")) {
		return diagnostic.New("E_CLI_USAGE", "-fx-max-age must be positive and requires a live FX source")
	}
	if c.fxProvider != "" && c.fxProvider != "alpha-vantage" {
		return diagnostic.New("E_CLI_USAGE", "unknown FX provider; use alpha-vantage or -fx-url")
	}
	return nil
}

func (c command) quoteProvider(limits config.Limits) (fx.Provider, error) {
	switch {
	case c.fxFixed != "":
		snapshot, err := c.loadSuppliedQuotes(limits)
		if err != nil {
			return nil, err
		}
		return snapshot, nil
	case c.fxURL != "":
		return fx.NewHTTP(c.fxURL, nil, c.fxPolicy)
	case c.fxProvider == "alpha-vantage":
		return fx.NewAlphaVantage(os.Getenv("FINLANG_ALPHA_VANTAGE_API_KEY"), nil, c.fxPolicy)
	default:
		return nil, nil
	}
}

func (c command) loadSuppliedQuotes(limits config.Limits) (*fx.Snapshot, error) {
	path := c.fxFixed
	if c.fxSnapshot != "" {
		path = c.fxSnapshot
	}
	if path == "" {
		return nil, nil
	}
	data, err := readLimitedFile(path, limits)
	if err != nil {
		return nil, err
	}
	if c.fxSnapshot != "" {
		return fx.LoadSnapshot(bytes.NewReader(data))
	}
	return fx.LoadFixed(bytes.NewReader(data))
}

// ruleArguments matches the decoded contract values to the named declarations.
func ruleArguments(rule *ast.Rule, spec contract.Spec, args []eval.Value) ([]eval.Value, error) {
	order, err := ruleInputOrder(rule, spec)
	if err != nil {
		return nil, err
	}
	ordered := make([]eval.Value, len(order))
	for i, index := range order {
		ordered[i] = args[index]
	}
	return ordered, nil
}

func ruleInputOrder(rule *ast.Rule, spec contract.Spec) ([]int, error) {
	if spec.Entry != rule.Name {
		return nil, diagnostic.WithSpan(diagnostic.New("E_CONTRACT_ENTRY", "contract entry must name the declared rule"), rule.Span)
	}
	if len(spec.Parameters) != len(rule.Inputs) {
		return nil, diagnostic.WithSpan(diagnostic.New("E_CONTRACT", "contract parameter count must match the rule input declarations"), rule.Span)
	}
	positions := make(map[string]int, len(spec.Parameters))
	for i, parameter := range spec.Parameters {
		positions[parameter.Name] = i
	}
	order := make([]int, 0, len(rule.Inputs))
	for _, input := range rule.Inputs {
		index, ok := positions[input.Name]
		if !ok || spec.Parameters[index].Type != input.Type || spec.Parameters[index].Currency != input.Currency {
			message := fmt.Sprintf("contract parameter must match the name, type and currency of rule input %q", input.Name)
			return nil, diagnostic.WithSpan(diagnostic.New("E_CONTRACT", message), input.Span)
		}
		order = append(order, index)
	}
	outputType := rule.Output.Type
	if outputType == "" {
		outputType = "money"
	}
	if spec.Output.Type != outputType || spec.Output.Currency != rule.Output.Currency {
		return nil, diagnostic.WithSpan(diagnostic.New("E_CONTRACT", "contract output type and currency must match the rule output declaration"), rule.Output.Span)
	}
	return order, nil
}
