# v0.2 trace module report

Scope: `internal/trace/**` only, plus this report. Evaluator instrumentation,
CLI handling, and the JSON tags on `diagnostic.Span` and `config.Limits` belong
to the coordinating implementation.

## Implemented behavior

- `NewRecorder`, `Record`, `Events`, and `Truncated` retain the first N events,
  assign contiguous one-based sequence numbers, and mark truncation only on an
  attempted record beyond the cap. Nonpositive caps retain no events.
- Mutable `Inputs` slices and `TargetScale` pointers are copied both when
  recording and when returning snapshots.
- `Header` exposes the agreed six fields with lower snake case JSON tags.
  An omitted schema version defaults to `"1"` without changing the caller's
  header, and nil input is encoded as JSON null.
- `WriteJSONL` writes one header, the retained events, and one summary in stable
  order. Each line has exactly one `record` discriminator. Events include false
  `inexact`; optional currency, target scale, mode, and branch are omitted when
  absent. A pointer to target scale zero is retained.
- Invalid raw header JSON is rejected before any bytes are written. Header,
  event, and summary writer errors are propagated; short writes return
  `io.ErrShortWrite`. No timestamps, process IDs, randomness, or runtime
  dependencies were added.

## Test-first evidence

1. Added recorder and JSONL tests before implementation.
2. `go test ./internal/trace` exited 1 with undefined `trace.NewRecorder`,
   `trace.Header`, and `trace.WriteJSONL`, establishing the missing-API RED.
3. Implemented only the assigned trace APIs and event tags.
4. `go test ./internal/trace -run 'TestRecorder|TestWriteJSONLNilInputAndEmptyRecorder|TestWriteJSONLRejectsMalformedInput|TestWriteJSONLPropagatesWriterFailures|TestWriteJSONLRejectsShortWrite' -count=1`
   passed. Tests exercise caps 3 and 10,000; zero and negative caps; sequence
   ownership; bidirectional deep copies; nil input; custom schema versions;
   malformed input; and failures on each record type.

## Integration checkpoint

At the first checkpoint, `go test ./internal/trace` failed only
`TestWriteJSONLDeterministicSchema`, because the shared `diagnostic.Span` and
`config.Limits` types still encoded their exported Go field names. The trace
test deliberately expects the agreed lowercase JSON schema, so those root-owned
tag changes are required before the final GREEN check.

At the same checkpoint, `go test ./...` also reported ongoing integration
failures outside this module: `TestSpanJSONContract` and the evaluator tests
`TestMoneyExpressions`, `TestMoneySettlementTiming`, and `TestMoneyErrors`
(Money builtins were not registered yet). These failures were reported to the
coordinator and were not modified from this task.

## Final owned validation

After the coordinator added lowercase JSON tags to `diagnostic.Span` and
`config.Limits`, all trace tests passed, including the exact deterministic JSONL
fixture. Fresh checks performed by this task:

- `go test ./internal/trace -count=1` — passed.
- `go test -race ./internal/trace -count=1` — passed.
- `go vet ./internal/trace` — passed with no output.

The earlier RED and integration checkpoint above are retained as historical
evidence, not current trace failures. The coordinator owns final evaluator, CLI,
and whole-repository verification.
