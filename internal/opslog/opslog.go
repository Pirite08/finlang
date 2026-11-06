// Package opslog writes operational records with a fixed set of fields.
package opslog

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"io"
	"time"

	"github.com/Pirite08/finlang/internal/diagnostic"
)

type record struct {
	SchemaVersion string          `json:"schema_version"`
	RunID         string          `json:"run_id"`
	Timestamp     time.Time       `json:"timestamp"`
	Status        string          `json:"status"`
	ExitCode      int             `json:"exit_code"`
	ErrorCode     diagnostic.Code `json:"error_code,omitempty"`
}

// CanAppend checks the format at the start and the line boundary at the end.
// This prevents accidentally reusing an audit file as an operational log.
func CanAppend(r io.ReaderAt, size int64) bool {
	if size == 0 {
		return true
	}
	var first [1024]byte
	n, err := r.ReadAt(first[:], 0)
	if err != nil && err != io.EOF {
		return false
	}
	line, _, found := bytes.Cut(first[:n], []byte{'\n'})
	var entry record
	if !found || jsonv2.Unmarshal(line, &entry, jsonv2.RejectUnknownMembers(true)) != nil ||
		entry.SchemaVersion != "1" || entry.RunID == "" || entry.Timestamp.IsZero() ||
		(entry.Status != "ok" && entry.Status != "error") {
		return false
	}
	var tail [1]byte
	_, err = r.ReadAt(tail[:], size-1)
	return err == nil && tail[0] == '\n'
}

// Write emits one JSON line. The caller supplies a generated run ID and a
// stable diagnostic code, without error messages or calculation data.
func Write(w io.Writer, runID string, at time.Time, exitCode int, errorCode diagnostic.Code) error {
	status := "ok"
	if exitCode == 0 {
		errorCode = ""
	} else {
		status = "error"
	}
	data, err := json.Marshal(record{SchemaVersion: "1", RunID: runID, Timestamp: at.UTC(), Status: status, ExitCode: exitCode, ErrorCode: errorCode})
	if err != nil {
		return err
	}
	line := append(data, '\n')
	n, err := w.Write(line)
	if err != nil {
		return err
	}
	if n != len(line) {
		return io.ErrShortWrite
	}
	return nil
}
