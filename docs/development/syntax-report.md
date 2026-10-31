# v0.1 lexer / AST / Pratt parser implementation report

Scope: Task 5–6 in `docs/development/syntax-brief.md`. Owned changes are limited to `internal/token`, `internal/lexer`, `internal/ast`, `internal/parser`, and this report. No commit was created.

## Integration interfaces

- `token.Kind` and `token.Token{Kind, Literal, Span}`.
- `lexer.New(file, source string, limits config.Limits) (*Lexer, error)` and `(*Lexer).Next() (token.Token, error)`.
- `ast.Node` exposes `GetSpan() diagnostic.Span`; `ast.Expr` and `ast.Stmt` retain private marker methods. All requested structs, exact field names, and pointer receivers are present. `ast.Block` is a Node, and `ast.Program` has `Statements []Stmt`.
- `parser.Parse(file, source string, limits config.Limits) (*ast.Program, error)` returns a nil program on error.

## Implemented behavior

- Source is validated as UTF-8. Spans use byte offsets, with one-based line and Unicode-rune column positions; Korean comments/strings work, and identifiers remain ASCII.
- Whitespace and `//` comments are skipped. A standalone `/` is preserved as a lexer token; the parser reports `E_DIVISION_POLICY_REQUIRED`, includes the token's source span, and directs the caller to `div(value, divisor, scale, "MODE")`.
- Numeric token text is preserved verbatim, including `0.100`. The lexer never parses numbers through `float64`.
- Double-quoted strings decode escaped double quotes, backslashes, `\n`, and `\t`. Unsupported escapes and missing closing quotes are diagnostics. Literal source newlines within strings are rejected; the newline escape is supported.
- All specified keywords, arithmetic/comparison operators, delimiters, and punctuation are recognized.
- Pratt precedence is calls, prefix `-`/`!`, multiplication, addition/subtraction, ordered comparisons, equality. Binary operators at the same level associate left.
- Function expressions, positional calls, let, return, if/else statements, and blocks are parsed. If is a statement; `else if` is not an additional grammar production. Trailing commas are rejected.
- Let, return, and expression statements require a semicolon between statements. The semicolon may be omitted before EOF or `}`. If/else blocks are compound statements and can be followed by another statement directly; an explicit semicolon after the compound statement is also accepted.
- Defaults come from `config.DefaultLimits()` when the corresponding supplied source/token/depth limit is nonpositive. Source size, numeric/identifier token bytes, and string payload bytes are bounded. String quotes are delimiters; the 256-byte string payload limit bounds both source bytes between quotes and decoded bytes.
- Recursive syntax depth is bounded. Tree heights are checked independently so long left-associated binary expressions and repeated calls cannot bypass the bound through Pratt's iterative loop. The structural height includes statement/block nodes, so nested structural combinations may reach the limit before 128 repeated operators.

## Test-first evidence

Tests were written against the required public APIs before the behavior implementation. Minimal API stubs then allowed a behavioral RED run rather than relying only on missing-package compilation errors.

Command: `go test ./internal/lexer ./internal/parser` (initial RED, exit 1).

- `TestTokenizeProgram`: got 1 token (the stub emitted only EOF).
- `TestNumericLiteralPreserved`: got an empty literal.
- `TestUTF8CommentPosition`: got no diagnostic.
- `TestLexerLimitsAndEncoding`: limit/encoding/escape checks got no diagnostics.
- `TestPrecedence`: got 0 statements.
- The initial `TestParseFunctionsAndIf` assertion panicked while indexing the empty stub program; this was test-assertion behavior during RED, not a production parser panic claim.

After implementation: `go test ./internal/lexer ./internal/parser` passed. Additional seed/boundary tests passed after formatting.

## Verification results

| Verification | Result |
| --- | --- |
| `go test ./internal/lexer ./internal/parser` | PASS |
| `go test -race ./internal/lexer ./internal/parser` | PASS |
| `go test ./internal/lexer -run=^$ -fuzz=FuzzLexerNoPanic -fuzztime=5s -parallel=2` | PASS, 170,198 executions; actual reported fuzz elapsed about 6 seconds |
| `go test ./internal/parser -run=^$ -fuzz=FuzzParserNoPanic -fuzztime=5s -parallel=2` | PASS, 220,596 executions; actual reported fuzz elapsed about 6 seconds |

Fuzz inputs are capped at 4,096 source bytes, 128 token bytes, and (for the parser) 32 syntax levels. Seeds include long numbers, unterminated strings, invalid UTF-8, Korean comments, deeply nested grouping, long left-associated chains, and repeated call chains. The lexer fuzz test also checks span ordering/bounds and termination within a source-length bound. These finite campaigns do not prove correctness for every input.

An integration snapshot of `go test ./...` was also run while other lanes were in progress. Lexer/parser passed, but the aggregate command failed because `internal/eval` had no non-test Go files, causing cmd/fin, cli, eval, and repl build failures, and Decimal's `TestRoundingOracleAgreement` could not open `testdata/rounding_oracle.json`. These were reported to the root agent; they are outside this lane. A later aggregate result, if available, belongs in the final integrated report.
