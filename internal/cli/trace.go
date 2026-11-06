package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/trace"
)

func validateLogDestination(destination string, inputs []string, code diagnostic.Code) error {
	if destination == "" {
		return nil
	}
	target, err := filepath.Abs(destination)
	if err != nil {
		return diagnostic.New(code, err.Error())
	}
	targetInfo, targetErr := os.Stat(destination)
	for _, input := range inputs {
		if input == "" {
			continue
		}
		path, err := filepath.Abs(input)
		if err != nil {
			return diagnostic.New(code, err.Error())
		}
		equal := target == path
		if runtime.GOOS == "windows" {
			equal = strings.EqualFold(target, path)
		}
		if info, err := os.Stat(input); err == nil && targetErr == nil && os.SameFile(targetInfo, info) {
			equal = true
		}
		if equal {
			return diagnostic.New(code, "log output must have a different path from inputs and other logs")
		}
	}
	return nil
}

func writeTrace(path string, header trace.Header, recorder *trace.Recorder, inputs []string, audit bool) error {
	code := diagnostic.Code("E_TRACE_IO")
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	dirMode := os.FileMode(0755)
	if audit {
		code = "E_AUDIT_IO"
		flags = os.O_CREATE | os.O_WRONLY | os.O_EXCL
		dirMode = 0700
	}
	if err := validateLogDestination(path, inputs, code); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return diagnostic.New(code, err.Error())
	}
	file, err := os.OpenFile(path, flags, 0600)
	if err != nil {
		return diagnostic.New(code, err.Error())
	}
	writeErr := trace.WriteJSONL(file, header, recorder)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		if audit {
			os.Remove(path)
		}
		if writeErr != nil {
			return diagnostic.New(code, writeErr.Error())
		}
		return diagnostic.New(code, closeErr.Error())
	}
	return nil
}
