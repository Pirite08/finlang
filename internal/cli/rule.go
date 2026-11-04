package cli

import (
	"bytes"
	"os"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
)

func (c command) hasFXSource() bool {
	return c.fxFixed != "" || c.fxURL != "" || c.fxProvider != ""
}

func (c command) validateFXFlags(ageSet bool) error {
	count := 0
	for _, source := range []string{c.fxFixed, c.fxURL, c.fxProvider} {
		if source != "" {
			count++
		}
	}
	if count > 1 {
		return diagnostic.New("E_CLI_USAGE", "choose one of -fx-fixed, -fx-url or -fx-provider")
	}
	if count > 0 && !c.json {
		return diagnostic.New("E_CLI_USAGE", "FX source options require -json rule execution")
	}
	if ageSet && (c.fxMaxAge <= 0 || (c.fxURL == "" && c.fxProvider == "")) {
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
		data, err := readLimitedFile(c.fxFixed, limits)
		if err != nil {
			return nil, err
		}
		return fx.LoadFixed(bytes.NewReader(data))
	case c.fxURL != "":
		return fx.NewHTTP(c.fxURL, nil, c.fxMaxAge)
	case c.fxProvider == "alpha-vantage":
		return fx.NewAlphaVantage(os.Getenv("FINLANG_ALPHA_VANTAGE_API_KEY"), nil, c.fxMaxAge)
	default:
		return nil, nil
	}
}

func validateRuleContract(rule *ast.Rule, spec contract.Spec) error {
	if spec.Entry != rule.Name {
		return diagnostic.WithSpan(diagnostic.New("E_CONTRACT_ENTRY", "contract entry must name the declared rule"), rule.Span)
	}
	if len(spec.Parameters) != 1 || spec.Parameters[0].Type != "money" ||
		spec.Parameters[0].Name != rule.Input.Name || spec.Parameters[0].Currency != rule.Input.Currency {
		return diagnostic.WithSpan(diagnostic.New("E_CONTRACT", "contract input must match the rule input name and currency"), rule.Input.Span)
	}
	if spec.Output.Type != "money" || spec.Output.Currency != rule.Output.Currency {
		return diagnostic.WithSpan(diagnostic.New("E_CONTRACT", "contract output currency must match the rule output declaration"), rule.Output.Span)
	}
	return nil
}
