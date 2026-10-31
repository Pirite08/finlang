package trace

import (
	"encoding/json"
	"io"

	"github.com/Pirite08/finlang/internal/config"
)

// Header identifies the reproducible inputs and settings for an execution.
type Header struct {
	SchemaVersion   string          `json:"schema_version"`
	ProgramSHA256   string          `json:"program_sha256"`
	ContractVersion string          `json:"contract_version"`
	RunnerVersion   string          `json:"runner_version"`
	Input           json.RawMessage `json:"input"`
	Limits          config.Limits   `json:"limits"`
}

// WriteJSONL writes a header, retained events, and a truncation summary.
// Invalid header input and writer errors are returned to the host CLI.
func WriteJSONL(w io.Writer, header Header, recorder *Recorder) error {
	if header.SchemaVersion == "" {
		header.SchemaVersion = "1"
	}
	if err := writeRecord(w, struct {
		Record string `json:"record"`
		Header
	}{Record: "header", Header: header}); err != nil {
		return err
	}
	for _, event := range recorder.events {
		if err := writeRecord(w, struct {
			Record string `json:"record"`
			Event
		}{Record: "event", Event: event}); err != nil {
			return err
		}
	}
	return writeRecord(w, struct {
		Record         string `json:"record"`
		EventsRecorded int    `json:"events_recorded"`
		TraceTruncated bool   `json:"trace_truncated"`
	}{Record: "summary", EventsRecorded: len(recorder.events), TraceTruncated: recorder.truncated})
}

func writeRecord(w io.Writer, record any) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	n, err := w.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}
