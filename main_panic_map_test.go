package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPanicStackTraceMapping(t *testing.T) {
	// 1. Create a temporary .srv file that triggers a division-by-zero panic
	tmpFile, err := os.CreateTemp("", "test_panic_*.srv")
	if err != nil {
		t.Fatalf("failed to create temp srv file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	srvContent := `
fn divide(a: int, b: int) -> int {
	return a / b
}

test "trigger panic" {
	let x = divide(10, 0)
}
`
	if _, err := tmpFile.WriteString(srvContent); err != nil {
		t.Fatalf("failed to write srv file: %v", err)
	}
	tmpFile.Close()

	// 2. Resolve the build directory for the temp srv file and find the compiler
	absPath, err := filepath.Abs(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	// 3. Build a unique compiler binary for testing to avoid collisions and flaky stderr pipes
	servBin := filepath.Join(os.TempDir(), fmt.Sprintf("serv_test_compiler_%d.exe", time.Now().UnixNano()))
	buildCmd := exec.Command("go", "build", "-o", servBin, ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build compiler binary for testing: %v", err)
	}
	defer os.Remove(servBin)

	// 4. Run the compiled serv binary with 'test' subcommand to execute the panicking test
	cmd := exec.Command(servBin, "test", absPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// We expect the command to fail because the test has a panic
	_ = cmd.Run()

	outputStderr := stderr.String()
	outputStdout := stdout.String()
	t.Logf("Compiler stdout output:\n%s", outputStdout)
	t.Logf("Compiler stderr output:\n%s", outputStderr)

	combinedOutput := outputStdout + "\n" + outputStderr

	// 5. Verify that the output stack trace was rewritten to point to the .srv file and correct line
	// The original divide function is on line 3, and the division 'a / b' is on line 3 of the srv file.
	// We want to ensure we see '<tmpFile_base>:<line>' and not 'service.go' or 'main.go'.
	expectedFile := filepath.Base(tmpFile.Name())
	if !strings.Contains(combinedOutput, expectedFile) {
		t.Errorf("Expected stack trace to contain original file %q, but it did not.\nCombined Output:\n%s", expectedFile, combinedOutput)
	}

	// Also make sure we mapped back to a line in the .srv file, e.g. line 3 or 7
	if !strings.Contains(combinedOutput, expectedFile+":3") && !strings.Contains(combinedOutput, expectedFile+":7") {
		t.Errorf("Expected stack trace to contain mapped line numbers (e.g. :3 or :7) for %q.\nCombined Output:\n%s", expectedFile, combinedOutput)
	}

	// Make sure service.go or main.go coordinate wasn't left untranslated for the user-defined stack frames
	// Note: system stack frames like runtime/panic.go are fine, but our generated files should be mapped.
	if strings.Contains(combinedOutput, "service.go:") {
		t.Errorf("Expected service.go references to be rewritten, but found 'service.go:' in Combined Output:\n%s", combinedOutput)
	}
}
