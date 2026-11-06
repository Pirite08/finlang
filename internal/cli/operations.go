package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/opslog"
)

func openOperations(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, diagnostic.New("E_OPS_LOG_IO", "could not prepare operational log directory")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return nil, diagnostic.New("E_OPS_LOG_IO", "could not open operational log")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !opslog.CanAppend(file, info.Size()) {
		file.Close()
		return nil, diagnostic.New("E_OPS_LOG_IO", "operational log must be an empty file or a complete operational JSONL log")
	}
	return file, nil
}

func (c command) finishOperation(file *os.File, runID string, code int) {
	writeErr := opslog.Write(file, runID, time.Now(), code, *c.operationError)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		// The result may already have reached stdout. Keep it intact and report
		// the operational logging failure separately without a path or raw error.
		fmt.Fprintln(c.stderr, "E_OPS_LOG_IO: could not finish operational log")
	}
}
