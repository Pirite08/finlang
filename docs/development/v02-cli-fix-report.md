# v0.2 CLI usage-error correction

Date: 2026-10-31.

Corrected the P2 option-order finding in `v02-final-review.md`. On a flag parse error, error output mode is recovered from explicit arguments, including flags beyond the first invalid option. The last valid explicit JSON boolean determines whether stdout receives one structured error record. Ordinary successful parsing and execution continue to use Go's flag parser.

The recovery walk consults registered flag definitions, consumes known string values (including values spelled `-json` or `--`), supports one/two leading hyphens and `=value`, uses `strconv.ParseBool` for every accepted Go boolean spelling, and stops at a standalone `--`, `-`, or first positional argument. Invalid boolean values do not independently request JSON. Source filenames equal to `-json` are therefore not interpreted as mode flags.

Changed paths:

- `internal/cli/cli.go`: parse-error-only JSON-intent recovery.
- `internal/cli/usage_test.go`: real `Run` behavior, exit 2, stderr presence, zero stdout when disabled, and exactly one `E_CLI_USAGE` JSON record when enabled. Tests use the existing decoder's EOF check to reject extra records.
- `docs/development/v02-cli-fix-report.md`: this report.

Validation commands and results:

1. `gofmt -w internal/cli/usage_test.go`
2. `go test ./internal/cli -run TestUsageErrorJSONIntent -count=1` — RED, exit 1. Expected failures included empty stdout/EOF with `-unknown -json`, `--json`, explicit true spellings, and unwanted JSON when a later explicit false should disable it.
3. Implemented recovery; `gofmt -w internal/cli/cli.go internal/cli/usage_test.go`.
4. `go test ./internal/cli -count=1` — GREEN, exit 0; `go vet ./internal/cli` — exit 0, no findings.
5. Added filename-before-error and consumed-terminator boundary cases; reran package tests, vet, and `go test ./... -count=1` successfully.
6. A test-only readability refactor briefly produced `type usageCase usageCase`; package test and vet both reported the invalid recursive type. Corrected immediately to the intended local struct.
7. Final `gofmt -w internal/cli/usage_test.go`; `go test ./internal/cli -count=1` — exit 0, CLI 1.063s; `go vet ./internal/cli` — exit 0, no findings; `go test ./... -count=1` — exit 0, all packages passed (CLI 0.931s).

No files outside the three owned paths were edited. No commit was created.
