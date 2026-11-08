package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
)

const serveTestRoute = `{"method":"POST","path":"/transfer-quotes","rule":"rules/fee.fin","contract":"rules/fee.json"}`

func TestLoadServeRoutes(t *testing.T) {
	dir := t.TempDir()
	rules := filepath.Join(dir, "rules")
	if err := os.Mkdir(rules, 0o700); err != nil {
		t.Fatal(err)
	}
	writeServeTestFile(t, filepath.Join(rules, "fee.fin"), `rule quote {
		input amount in "KRW"
		input multiplier as decimal
		output fee in "KRW"
		step calculate {
			require false
			let fee = amount * multiplier
		}
		return fee
	}`)
	contractPath := filepath.Join(rules, "fee.json")
	const quoteContract = `{
		"schema_version":"1","rule":"quote","entry":"quote",
		"parameters":[
			{"name":"multiplier","type":"decimal","min":"0","max":"1","scale_max":4},
			{"name":"amount","type":"money","currency":"KRW","min":"1","max":"1000000","scale_max":0}
		],
		"output":{"type":"money","currency":"KRW","min":"0","max":"1000000","scale":0}
	}`
	writeServeTestFile(t, contractPath, quoteContract)
	manifest := filepath.Join(dir, "app.json")
	writeServeTestFile(t, manifest, `{"schema_version":"1","routes":[`+serveTestRoute+`]}`)
	app, err := loadServeApp(manifest)
	if err != nil {
		t.Fatal(err)
	}
	route, ok := app.routes["/transfer-quotes"]
	if !ok || route.rule.Name != "quote" || len(route.inputOrder) != 2 ||
		route.inputOrder[0] != 1 || route.inputOrder[1] != 0 {
		t.Fatalf("unexpected route: %+v", route)
	}
	writeServeTestFile(t, contractPath, strings.Replace(quoteContract,
		`"output":{"type":"money","currency":"KRW"`, `"output":{"type":"money","currency":"USD"`, 1))
	if _, err := loadServeApp(manifest); err == nil || !strings.Contains(err.Error(), "output type and currency") {
		t.Fatalf("incompatible output accepted, error = %v", err)
	}
	writeServeTestFile(t, contractPath, quoteContract)
	writeServeTestFile(t, filepath.Join(rules, "fee.fin"), `rule quote {
		input amount in "KRW"
		input multiplier as decimal
		output fee in "KRW"
		step calculate { let fee = print(amount) }
		return fee
	}`)
	if _, err := loadServeApp(manifest); err == nil || !strings.Contains(err.Error(), "E_RULE_POLICY") {
		t.Fatalf("unsafe rule accepted, error = %v", err)
	}
}

func TestLoadServeRoutesAppliesCollectionProfile(t *testing.T) {
	rule, err := loadServedRule("../../examples/group_transactions_rule_v2.fin",
		"../../contracts/group_transactions_v2.json", config.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if rule.spec.SchemaVersion != "2" || rule.limits.MaxCollectionItems != config.MaxRuleCollectionItems {
		t.Fatalf("unexpected collection profile: %+v", rule)
	}
}

func TestLoadServeAppTransferVersions(t *testing.T) {
	for _, tc := range []struct{ path, version string }{
		{"../../examples/transfer_app.json", "1"},
		{"../../examples/transfer_app_v2.json", "2"},
	} {
		t.Run(tc.version, func(t *testing.T) {
			app, err := loadServeApp(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			route := app.routes["/transfers"]
			if route.ruleVersion != tc.version || route.contractArtifactVersion != tc.version ||
				len(app.planArtifacts) != 2 {
				t.Fatalf("unexpected route versions %q/%q and artifacts %d", route.ruleVersion, route.contractArtifactVersion, len(app.planArtifacts))
			}
			for _, artifact := range app.planArtifacts {
				if artifact.Version != tc.version || artifact.Rule != route.spec.Rule ||
					len(artifact.Data) == 0 || artifact.SHA256 != sha256Hex(artifact.Data) {
					t.Fatalf("invalid archived artifact %s %q", artifact.Kind, artifact.Version)
				}
			}
		})
	}
}

func TestLoadServeRoutesRejectsInvalidManifest(t *testing.T) {
	for name, body := range map[string]string{
		"duplicate field":          `{"schema_version":"1","schema_version":"1","routes":[` + serveTestRoute + `]}`,
		"unknown field":            `{"schema_version":"1","routes":[],"extra":1}`,
		"trailing value":           `{"schema_version":"1","routes":[` + serveTestRoute + `]} {}`,
		"empty routes":             `{"schema_version":"1","routes":[]}`,
		"wrong version":            `{"schema_version":"2","routes":[` + serveTestRoute + `]}`,
		"wrong method":             `{"schema_version":"1","routes":[` + strings.Replace(serveTestRoute, "POST", "GET", 1) + `]}`,
		"unclean path":             `{"schema_version":"1","routes":[` + strings.Replace(serveTestRoute, "/transfer-quotes", "/../transfer-quotes", 1) + `]}`,
		"wildcard path":            `{"schema_version":"1","routes":[` + strings.Replace(serveTestRoute, "/transfer-quotes", "/{name}", 1) + `]}`,
		"escaped path":             `{"schema_version":"1","routes":[` + strings.Replace(serveTestRoute, "/transfer-quotes", "/%74ransfer-quotes", 1) + `]}`,
		"duplicate route":          `{"schema_version":"1","routes":[` + serveTestRoute + "," + serveTestRoute + `]}`,
		"version outside transfer": `{"schema_version":"1","routes":[` + strings.Replace(serveTestRoute, `"method":`, `"rule_version":"1","method":`, 1) + `]}`,
		"invalid artifact version": `{"schema_version":"1","routes":[` + strings.Replace(serveTestRoute, `"method":`, `"action":"transfer","rule_version":"bad/version","method":`, 1) + `]}`,
		"root conflict":            `{"schema_version":"1","preview":"demo.html","routes":[` + strings.Replace(serveTestRoute, "/transfer-quotes", "/", 1) + `]}`,
	} {
		t.Run(name, func(t *testing.T) {
			manifest := filepath.Join(t.TempDir(), "app.json")
			writeServeTestFile(t, manifest, body)
			if _, err := loadServeApp(manifest); err == nil || !strings.Contains(err.Error(), "E_SERVE_CONFIG") {
				t.Fatalf("invalid manifest accepted, error = %v", err)
			}
		})
	}
}

func writeServeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
