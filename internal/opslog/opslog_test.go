package opslog

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"
)

func TestWriteOperationalRecords(t *testing.T) {
	at := time.Date(2026, 11, 5, 9, 30, 0, 0, time.FixedZone("KST", 9*60*60))
	var log bytes.Buffer
	if err := Write(&log, "run-one", at, 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := Write(&log, "run-two", at, 1, "E_FX_OUTLIER"); err != nil {
		t.Fatal(err)
	}
	want := `{"schema_version":"1","run_id":"run-one","timestamp":"2026-11-05T00:30:00Z","status":"ok","exit_code":0}` + "\n" +
		`{"schema_version":"1","run_id":"run-two","timestamp":"2026-11-05T00:30:00Z","status":"error","exit_code":1,"error_code":"E_FX_OUTLIER"}` + "\n"
	if got := log.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

type shortWriter struct{ calls int }

func (w *shortWriter) Write(p []byte) (int, error) {
	w.calls++
	return len(p) - 1, nil
}

func TestWriteReportsShortWriteWithoutSplittingRecord(t *testing.T) {
	w := &shortWriter{}
	if err := Write(w, "run-one", time.Now(), 0, ""); !errors.Is(err, io.ErrShortWrite) || w.calls != 1 {
		t.Fatalf("got error %v and %d calls, want short write after one call", err, w.calls)
	}
}
