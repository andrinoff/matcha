package macos

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed save_file_picker.swift
var saveFilePickerSwift string

// SaveFilePicker launches the native macOS save panel (NSSavePanel).
// It returns the selected save path, or empty string if the user cancelled.
func SaveFilePicker(initialPath string, suggestedFilename string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("SaveFilePicker is only supported on macOS")
	}

	tmpDir, err := os.MkdirTemp("", "matcha-savepicker")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck

	swiftFile := filepath.Join(tmpDir, "save_file_picker.swift")
	if err := os.WriteFile(swiftFile, []byte(saveFilePickerSwift), 0644); err != nil {
		return "", err
	}

	binFile := filepath.Join(tmpDir, "save_file_picker")

	// Compile
	cmd := exec.Command("swiftc", swiftFile, "-o", binFile) //nolint:noctx
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to compile save file picker helper: %w\n%s", err, string(out))
	}

	// Run
	args := []string{}
	if initialPath != "" {
		args = append(args, initialPath)
	}
	if suggestedFilename != "" {
		args = append(args, suggestedFilename)
	}
	out, err := exec.Command(binFile, args...).Output() //nolint:noctx
	if err != nil {
		// Exit code non-zero usually means user cancelled
		return "", nil
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return "", nil
	}

	return trimmed, nil
}
