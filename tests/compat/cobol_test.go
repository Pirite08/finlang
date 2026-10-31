package compat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

type commandRecord struct {
	Phase            string   `json:"phase"`
	Rule             string   `json:"rule,omitempty"`
	Executable       string   `json:"executable"`
	Arguments        []string `json:"arguments"`
	WorkingDirectory string   `json:"working_directory"`
	TimeoutSeconds   int      `json:"timeout_seconds"`
}

type fileHash struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type ruleSummary struct {
	Record                 string         `json:"record"`
	Rule                   string         `json:"rule"`
	ContractAccepted       int            `json:"contract_accepted"`
	ContractRejected       int            `json:"contract_rejected"`
	Executed               int            `json:"executed"`
	Matched                int            `json:"matched"`
	Mismatched             int            `json:"mismatched"`
	DSLErrors              int            `json:"dsl_errors"`
	COBOLErrors            int            `json:"cobol_errors"`
	ManualAnchors          int            `json:"manual_anchors"`
	ManualAnchorMismatches int            `json:"manual_anchor_mismatches"`
	Branches               map[string]int `json:"branches"`
}

// One sequential owner writes the optional report and compiles each reference
// once. Never use t.Parallel here: case and rule order form part of the evidence.
func TestCOBOLCompatibility(t *testing.T) {
	compiler := os.Getenv("COBC")
	if compiler == "" {
		compiler = "cobc"
	}
	compiler, err := exec.LookPath(compiler)
	if err != nil {
		message := fmt.Sprintf("COBOL comparison unexecuted: compiler unavailable (%v); set COBC to a GnuCOBOL executable", err)
		if os.Getenv("FINLANG_REQUIRE_COBOL") == "1" {
			t.Fatal(message)
		}
		t.Skip(message)
	}
	compiler, err = filepath.Abs(compiler)
	if err != nil {
		t.Fatal(err)
	}
	rules := make([]ruleRuntime, len(ruleNames))
	for i, name := range ruleNames {
		rules[i] = loadRule(t, name)
	}
	root := rules[0].root
	buildDir := t.TempDir()
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	commands := []commandRecord{{Phase: "version", Executable: compiler, Arguments: []string{"--version"}, WorkingDirectory: root, TimeoutSeconds: 10}}
	for _, name := range ruleNames {
		executable := filepath.Join(buildDir, name+suffix)
		commands = append(commands,
			// A relative ASCII source path also works when the checkout contains
			// Korean characters; native cobc 3.2 rejects that absolute path.
			commandRecord{Phase: "compile", Rule: name, Executable: compiler, Arguments: []string{"-x", "-free", "-std=cobol2014", "-freserved=NEAREST-EVEN", "-o", executable, filepath.Join("testdata", "cobol", name+".cob")}, WorkingDirectory: root, TimeoutSeconds: 60},
			commandRecord{Phase: "run", Rule: name, Executable: executable, Arguments: []string{}, WorkingDirectory: root, TimeoutSeconds: 30})
	}
	version, err := runCommand(commands[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	evidence := newEvidence(t)
	files := []fileHash{}
	for _, name := range ruleNames {
		for _, path := range []string{"examples/" + name + ".fin", "contracts/" + name + ".json", "testdata/cobol/" + name + ".cob"} {
			files = append(files, hashFile(t, root, path))
		}
	}
	harnessPaths, err := filepath.Glob(filepath.Join(root, "tests", "compat", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range harnessPaths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, hashFile(t, root, filepath.ToSlash(relative)))
	}
	compilerHash := hashFile(t, filepath.Dir(compiler), filepath.Base(compiler))
	evidence.write(t, map[string]any{
		"record": "header", "schema_version": "1", "seed": caseSeed,
		"seed_policy": "math/rand.NewSource(seed + rule index); rule order recorded below",
		"rule_order":  ruleNames, "accepted_per_rule": acceptedPerRule,
		"go_version": runtime.Version(), "goos": runtime.GOOS, "goarch": runtime.GOARCH,
		"compiler": compiler, "compiler_version": strings.TrimSpace(string(version)), "compiler_sha256": compilerHash.SHA256,
		"environment": compatibilityEnvironment(), "commands": commands, "files": files,
		"currency_policy": "COBOL records carry no currency token; its currency is the fixed output currency of the hashed contract; DSL currency is read from EncodeResult",
	})
	total := ruleSummary{Record: "total_summary", Rule: "all", Branches: map[string]int{}}
	for i, rule := range rules {
		compile, run := commands[1+i*2], commands[2+i*2]
		if _, err := runCommand(compile, nil); err != nil {
			evidence.write(t, map[string]any{"record": "infrastructure_error", "rule": rule.spec.Rule, "phase": "compile", "error": err.Error()})
			t.Fatal(err)
		}
		name := []string{"TestCOBOLInterest", "TestCOBOLFee", "TestCOBOLSum"}[i]
		t.Run(name, func(t *testing.T) {
			summary := compareRule(t, rule, run, evidence)
			total.ContractAccepted += summary.ContractAccepted
			total.ContractRejected += summary.ContractRejected
			total.Executed += summary.Executed
			total.Matched += summary.Matched
			total.Mismatched += summary.Mismatched
			total.DSLErrors += summary.DSLErrors
			total.COBOLErrors += summary.COBOLErrors
			total.ManualAnchors += summary.ManualAnchors
			total.ManualAnchorMismatches += summary.ManualAnchorMismatches
			for branch, count := range summary.Branches {
				total.Branches[branch] += count
			}
		})
	}
	evidence.write(t, total)
	t.Logf("total: accepted=%d executed=%d matched=%d mismatched=%d contract_rejected=%d; rejected inputs excluded from executed denominator", total.ContractAccepted, total.Executed, total.Matched, total.Mismatched, total.ContractRejected)
}

func compareRule(t *testing.T, rule ruleRuntime, command commandRecord, evidence *evidenceWriter) ruleSummary {
	t.Helper()
	cases := generatedCases(rule.spec)
	summary := ruleSummary{Record: "rule_summary", Rule: rule.spec.Rule, Branches: map[string]int{}}
	// Validate the complete batch before constructing subprocess stdin. This is
	// the only source of the executed batch; rejection cases never join it.
	var input bytes.Buffer
	fmt.Fprintln(&input, len(cases))
	for _, c := range cases {
		if _, err := contract.DecodeInput(bytes.NewReader(c.Input), rule.spec, config.DefaultLimits()); err != nil {
			t.Fatalf("generated input %s rejected: %v", c.ID, err)
		}
		summary.ContractAccepted++
		fmt.Fprintln(&input, c.ID+"|"+strings.Join(c.Args, "|"))
	}
	for _, c := range rejectedCases(rule.spec) {
		_, err := contract.DecodeInput(bytes.NewReader(c.Input), rule.spec, config.DefaultLimits())
		if diagnostic.CodeOf(err) != "E_CONTRACT_INPUT" {
			t.Fatalf("rejected input %s got %v", c.ID, err)
		}
		summary.ContractRejected++
		evidence.write(t, map[string]any{"record": "contract_rejected", "rule": rule.spec.Rule, "id": c.ID, "label": c.Label, "input_json": string(c.Input), "error_class": "E_CONTRACT_INPUT", "executed": false})
	}
	stdout, err := runCommand(command, input.Bytes())
	if err != nil {
		evidence.write(t, map[string]any{"record": "infrastructure_error", "rule": rule.spec.Rule, "phase": "run", "error": err.Error()})
		t.Fatal(err)
	}
	cobol, err := parseCOBOL(stdout, cases, rule.spec)
	if err != nil {
		evidence.write(t, map[string]any{"record": "infrastructure_error", "rule": rule.spec.Rule, "phase": "protocol", "error": err.Error()})
		t.Fatal(err)
	}
	for i, c := range cases {
		dsl, err := rule.execute(c.Input)
		if err != nil {
			t.Fatalf("DSL %s: %v", c.ID, err)
		}
		summary.Executed++
		summary.Branches[dsl.Branch]++
		if dsl.Status == "error" {
			summary.DSLErrors++
		}
		if cobol[i].Status == "error" {
			summary.COBOLErrors++
		}
		difference := compareOutcomes(dsl, cobol[i])
		anchorMatch := true
		if c.Want != nil {
			summary.ManualAnchors++
			anchorMatch = compareOutcomes(dsl, *c.Want) == nil && compareOutcomes(cobol[i], *c.Want) == nil
			if !anchorMatch {
				summary.ManualAnchorMismatches++
			}
		}
		match := difference == nil && anchorMatch
		if match {
			summary.Matched++
		} else {
			summary.Mismatched++
			t.Errorf("%s %s input=%s DSL=%+v COBOL=%+v manual=%+v", c.ID, c.Label, c.Input, dsl, cobol[i], c.Want)
		}
		record := map[string]any{"record": "case", "rule": rule.spec.Rule, "id": c.ID, "label": c.Label, "arguments": c.Args, "input": c.Input, "dsl": dsl, "cobol": cobol[i], "differential_match": difference == nil, "match": match}
		if c.Want != nil {
			record["manual_anchor"], record["manual_anchor_match"] = c.Want, anchorMatch
		}
		evidence.write(t, record)
	}
	evidence.write(t, summary)
	t.Logf("%s: accepted=%d executed=%d matched=%d mismatched=%d DSL_errors=%d COBOL_errors=%d anchors=%d contract_rejected=%d branches=%v", summary.Rule, summary.ContractAccepted, summary.Executed, summary.Matched, summary.Mismatched, summary.DSLErrors, summary.COBOLErrors, summary.ManualAnchors, summary.ContractRejected, summary.Branches)
	return summary
}

// Direct argument vectors only: no shell, command concatenation, or quoting.
func runCommand(command commandRecord, input []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(command.TimeoutSeconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command.Executable, command.Arguments...)
	cmd.Dir = command.WorkingDirectory
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s %s timed out: %w", command.Phase, command.Rule, ctx.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("%s %s failed: %w; stderr=%q; stdout=%q", command.Phase, command.Rule, err, stderr.String(), stdout.String())
	}
	if stderr.Len() != 0 {
		return nil, fmt.Errorf("%s %s wrote unexpected stderr: %q", command.Phase, command.Rule, stderr.String())
	}
	return stdout.Bytes(), nil
}

type evidenceWriter struct {
	encoder *json.Encoder
}

func newEvidence(t *testing.T) *evidenceWriter {
	t.Helper()
	var writer io.Writer = io.Discard
	if path := os.Getenv("FINLANG_COMPAT_REPORT"); path != "" {
		if !filepath.IsAbs(path) {
			t.Fatal("FINLANG_COMPAT_REPORT must be an absolute path")
		}
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := file.Close(); err != nil {
				t.Error(err)
			}
		})
		writer = file
		t.Logf("compatibility evidence: %s", path)
	}
	return &evidenceWriter{encoder: json.NewEncoder(writer)}
}

func (w *evidenceWriter) write(t *testing.T, value any) {
	t.Helper()
	if err := w.encoder.Encode(value); err != nil {
		t.Fatal(err)
	}
}

func hashFile(t *testing.T, root, path string) fileHash {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	return fileHash{Path: path, SHA256: hex.EncodeToString(hash[:])}
}

func compatibilityEnvironment() map[string]string {
	result := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if ok && (strings.HasPrefix(upper, "COB") || upper == "PATH" || upper == "CC" || upper == "FINLANG_REQUIRE_COBOL") {
			result[key] = value
		}
	}
	return result
}
