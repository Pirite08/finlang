package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/trace"
)

func validateTraceDestination(destination string, inputs []string) error {
	if destination == "" {
		return nil
	}
	target, err := filepath.Abs(destination)
	if err != nil {
		return diagnostic.New("E_TRACE_IO", err.Error())
	}
	targetInfo, targetErr := os.Stat(destination)
	for _, input := range inputs {
		if input == "" {
			continue
		}
		path, err := filepath.Abs(input)
		if err != nil {
			return diagnostic.New("E_TRACE_IO", err.Error())
		}
		equal := target == path
		if runtime.GOOS == "windows" {
			equal = strings.EqualFold(target, path)
		}
		if info, err := os.Stat(input); err == nil && targetErr == nil && os.SameFile(targetInfo, info) {
			equal = true
		}
		if equal {
			return diagnostic.New("E_TRACE_IO", "trace output must not overwrite the source, contract, input or fixed quote file")
		}
	}
	return nil
}

func writeTrace(path string, header trace.Header, recorder *trace.Recorder, inputs []string) error {
	if err := validateTraceDestination(path, inputs); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return diagnostic.New("E_TRACE_IO", err.Error())
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return diagnostic.New("E_TRACE_IO", err.Error())
	}
	writeErr := trace.WriteJSONL(file, header, recorder)
	closeErr := file.Close()
	if writeErr != nil {
		return diagnostic.New("E_TRACE_IO", writeErr.Error())
	}
	if closeErr != nil {
		return diagnostic.New("E_TRACE_IO", closeErr.Error())
	}
	return nil
}
