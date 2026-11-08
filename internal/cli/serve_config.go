package cli

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"unicode/utf8"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/parser"
	"github.com/Pirite08/finlang/internal/transfer"
)

const maxServeRoutes = 64

var (
	staticServePath = regexp.MustCompile(`^/(?:[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*)?$`)
	artifactVersion = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,95}$`)
)

type serveManifest struct {
	SchemaVersion string           `json:"schema_version"`
	Preview       string           `json:"preview,omitempty"`
	Routes        []serveRouteSpec `json:"routes"`
}

type serveRouteSpec struct {
	Method          string `json:"method"`
	Path            string `json:"path"`
	Rule            string `json:"rule"`
	Contract        string `json:"contract"`
	Action          string `json:"action,omitempty"`
	RuleVersion     string `json:"rule_version,omitempty"`
	ContractVersion string `json:"contract_version,omitempty"`
}

type servedRule struct {
	rule                                 *ast.Rule
	spec                                 contract.Spec
	limits                               config.Limits
	inputOrder                           []int
	transfer                             bool
	programHash, contractHash            string
	ruleVersion, contractArtifactVersion string
	source, metadata                     []byte
}

type servedApp struct {
	routes        map[string]servedRule
	preview       []byte
	planArtifacts []transfer.PlanArtifact
	ledger        *transfer.Store
}

func loadServeApp(manifestPath string) (servedApp, error) {
	limits := config.DefaultLimits()
	data, err := readLimitedFile(manifestPath, limits)
	if err != nil {
		return servedApp{}, err
	}
	var manifest serveManifest
	if err := jsonv2.Unmarshal(data, &manifest, jsonv2.RejectUnknownMembers(true)); err != nil {
		return servedApp{}, diagnostic.New("E_SERVE_CONFIG", err.Error())
	}
	if manifest.SchemaVersion != "1" || len(manifest.Routes) == 0 || len(manifest.Routes) > maxServeRoutes {
		return servedApp{}, diagnostic.New("E_SERVE_CONFIG", fmt.Sprintf("expected schema version 1 and 1 to %d routes", maxServeRoutes))
	}
	routes := make(map[string]servedRule, len(manifest.Routes))
	for _, route := range manifest.Routes {
		if route.Method != http.MethodPost || !staticServePath.MatchString(route.Path) || path.Clean(route.Path) != route.Path ||
			route.Rule == "" || route.Contract == "" {
			return servedApp{}, diagnostic.New("E_SERVE_CONFIG", fmt.Sprintf("route %q requires POST, an exact static path, a rule and a contract", route.Path))
		}
		if route.Action != "" && route.Action != "transfer" {
			return servedApp{}, diagnostic.New("E_SERVE_CONFIG", fmt.Sprintf("route %q has an unsupported action", route.Path))
		}
		for _, version := range []string{route.RuleVersion, route.ContractVersion} {
			if version != "" && (route.Action != "transfer" || !artifactVersion.MatchString(version)) {
				return servedApp{}, diagnostic.New("E_SERVE_CONFIG", fmt.Sprintf("route %q has an invalid transfer artifact version", route.Path))
			}
		}
		if _, duplicate := routes[route.Path]; duplicate {
			return servedApp{}, diagnostic.New("E_SERVE_CONFIG", fmt.Sprintf("duplicate route %q", route.Path))
		}
		routes[route.Path] = servedRule{}
	}
	dir := filepath.Dir(manifestPath)
	app := servedApp{routes: routes}
	if manifest.Preview != "" {
		if _, conflict := routes["/"]; conflict {
			return servedApp{}, diagnostic.New("E_SERVE_CONFIG", "preview reserves the root route")
		}
		app.preview, err = readLimitedFile(serveAssetPath(dir, manifest.Preview), limits)
		if err != nil {
			return servedApp{}, fmt.Errorf("preview: %w", err)
		}
		if len(app.preview) == 0 || !utf8.Valid(app.preview) {
			return servedApp{}, diagnostic.New("E_SERVE_CONFIG", "preview must be nonempty UTF-8 HTML")
		}
	}
	for _, route := range manifest.Routes {
		loaded, err := loadServedRule(serveAssetPath(dir, route.Rule), serveAssetPath(dir, route.Contract), limits)
		if err != nil {
			return servedApp{}, fmt.Errorf("route %q: %w", route.Path, err)
		}
		if route.Action == "transfer" {
			if err := validateTransferRoute(loaded); err != nil {
				return servedApp{}, fmt.Errorf("route %q: %w", route.Path, err)
			}
			loaded.transfer = true
			if route.RuleVersion != "" {
				loaded.ruleVersion = route.RuleVersion
			}
			if route.ContractVersion != "" {
				loaded.contractArtifactVersion = route.ContractVersion
			}
			app.planArtifacts = append(app.planArtifacts,
				transfer.PlanArtifact{Kind: "rule", Rule: loaded.spec.Rule, Version: loaded.ruleVersion, SHA256: loaded.programHash, Data: loaded.source},
				transfer.PlanArtifact{Kind: "contract", Rule: loaded.spec.Rule, Version: loaded.contractArtifactVersion, SHA256: loaded.contractHash, Data: loaded.metadata},
			)
		}
		loaded.source, loaded.metadata = nil, nil
		routes[route.Path] = loaded
	}
	return app, nil
}

func serveAssetPath(dir, name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(dir, name)
}

func loadServedRule(rulePath, contractPath string, limits config.Limits) (servedRule, error) {
	metadata, err := readLimitedFile(contractPath, limits)
	if err != nil {
		return servedRule{}, err
	}
	source, err := readLimitedFile(rulePath, limits)
	if err != nil {
		return servedRule{}, err
	}
	return prepareServedRule(rulePath, source, metadata, limits)
}

func prepareServedRule(rulePath string, source, metadata []byte, limits config.Limits) (servedRule, error) {
	spec, err := contract.Load(bytes.NewReader(metadata), limits)
	if err != nil {
		return servedRule{}, err
	}
	collections := spec.SchemaVersion != "1"
	if collections {
		limits = config.RuleCollectionLimits(limits)
	}
	program, err := parser.Parse(rulePath, string(source), limits)
	if err != nil {
		return servedRule{}, err
	}
	rule := program.Rule
	if rule == nil {
		return servedRule{}, diagnostic.New("E_RULE_POLICY", "serve routes require a rule declaration")
	}
	if spec.Rule != rule.Name {
		return servedRule{}, diagnostic.New("E_CONTRACT_ENTRY", "contract rule name must match the declared rule")
	}
	order, err := ruleInputOrder(rule, spec)
	if err != nil {
		return servedRule{}, err
	}
	evaluator := eval.New(eval.Options{Limits: limits, RuleOnly: true, CollectionRules: collections})
	if err := evaluator.ValidateRulePolicy(rule); err != nil {
		return servedRule{}, err
	}
	programHash, contractHash := sha256Hex(source), sha256Hex(metadata)
	return servedRule{
		rule: rule, spec: spec, limits: limits, inputOrder: order,
		programHash: programHash, contractHash: contractHash,
		ruleVersion: "sha256-" + programHash, contractArtifactVersion: "sha256-" + contractHash,
		source: source, metadata: metadata,
	}, nil
}
