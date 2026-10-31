package trace_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/trace"
)

func TestWriteJSONLDeterministicSchema(t *testing.T) {
	scale := uint32(0)
	recorder := trace.NewRecorder(2)
	recorder.Record(trace.Event{Kind: "round", Span: diagnostic.Span{File: "rule.fin", Start: 10, End: 20, Line: 3, Column: 2}, Inputs: []string{"1.5"}, Result: "2", Currency: "KRW", TargetScale: &scale, Mode: "HALF_EVEN", Inexact: true})
	recorder.Record(trace.Event{Kind: "branch", Inputs: []string{"true"}, Result: "true", Branch: "then"})
	recorder.Record(trace.Event{Kind: "discarded"})
	header := trace.Header{ProgramSHA256: "abc", ContractVersion: "1", RunnerVersion: "0.2", Input: json.RawMessage(`{"amount":"1.5"}`), Limits: config.Limits{MaxTraceEvents: 2}}
	want := `{"record":"header","schema_version":"1","program_sha256":"abc","contract_version":"1","runner_version":"0.2","input":{"amount":"1.5"},"limits":{"max_coefficient_digits":0,"max_scale":0,"max_token_bytes":0,"max_source_bytes":0,"max_syntax_depth":0,"max_eval_steps":0,"max_call_depth":0,"max_trace_events":2}}
{"record":"event","sequence":1,"kind":"round","source":{"file":"rule.fin","start":10,"end":20,"line":3,"column":2},"inputs":["1.5"],"result":"2","currency":"KRW","target_scale":0,"mode":"HALF_EVEN","inexact":true}
{"record":"event","sequence":2,"kind":"branch","source":{"file":"","start":0,"end":0,"line":0,"column":0},"inputs":["true"],"result":"true","inexact":false,"branch":"then"}
{"record":"summary","events_recorded":2,"trace_truncated":true}
`
	for i := 0; i < 2; i++ {
		var output bytes.Buffer
		if err := trace.WriteJSONL(&output, header, recorder); err != nil {
			t.Fatal(err)
		}
		if output.String() != want {
			t.Fatalf("JSONL differs:\ngot:  %s\nwant: %s", output.String(), want)
		}
	}
	if header.SchemaVersion != "" {
		t.Fatal("WriteJSONL mutated header")
	}
}

func TestWriteJSONLNilInputAndEmptyRecorder(t *testing.T) {
	var output bytes.Buffer
	if err := trace.WriteJSONL(&output, trace.Header{SchemaVersion: "custom"}, trace.NewRecorder(1)); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"schema_version":"custom"`) || !strings.Contains(lines[0], `"input":null`) || lines[1] != `{"record":"summary","events_recorded":0,"trace_truncated":false}` {
		t.Fatalf("unexpected empty trace: %s", output.String())
	}
}

func TestWriteJSONLRejectsMalformedInput(t *testing.T) {
	var output bytes.Buffer
	if err := trace.WriteJSONL(&output, trace.Header{Input: json.RawMessage(`{"broken":`)}, trace.NewRecorder(1)); err == nil {
		t.Fatal("malformed header input accepted")
	}
	if output.Len() != 0 {
		t.Fatalf("wrote %q before detecting invalid header", output.String())
	}
}

type failAfterWriter struct {
	remaining int
	err       error
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		return 0, w.err
	}
	w.remaining--
	return len(p), nil
}

func TestWriteJSONLPropagatesWriterFailures(t *testing.T) {
	sentinel := errors.New("trace disk full")
	recorder := trace.NewRecorder(1)
	recorder.Record(trace.Event{Kind: "add"})
	for _, after := range []int{0, 1, 2} {
		writer := &failAfterWriter{remaining: after, err: sentinel}
		if err := trace.WriteJSONL(writer, trace.Header{}, recorder); !errors.Is(err, sentinel) {
			t.Fatalf("after %d records: got %v, want writer error", after, err)
		}
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestWriteJSONLRejectsShortWrite(t *testing.T) {
	if err := trace.WriteJSONL(shortWriter{}, trace.Header{}, trace.NewRecorder(1)); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("got %v, want io.ErrShortWrite", err)
	}
}
