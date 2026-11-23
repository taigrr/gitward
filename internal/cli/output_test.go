package cli

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/fang"
)

func TestErrorHandlerJSONWriteFailure(t *testing.T) {
	// A read-only stderr deterministically rejects the JSON envelope.
	file, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	originalStderr, originalJSON := os.Stderr, jsonFlag
	os.Stderr, jsonFlag = file, true
	t.Cleanup(func() { os.Stderr, jsonFlag = originalStderr, originalJSON })
	var output bytes.Buffer
	ErrorHandler(&output, fang.Styles{}, errors.New("original failure"))
	for _, want := range []string{"original failure", "writing JSON error"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("fallback output %q missing %q", output.String(), want)
		}
	}
}
