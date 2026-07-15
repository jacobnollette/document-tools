// Package ocr extracts text from uploaded documents and runs the async
// processing queue that moves documents from pending to completed.
package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Engine extracts text from a document file on disk.
type Engine interface {
	// Extract returns the recognized text for the file at path.
	Extract(ctx context.Context, path string) (string, error)
}

// Tesseract runs the tesseract CLI against image files.
type Tesseract struct {
	// Binary is the tesseract executable name or path. Defaults to "tesseract".
	Binary string
	// Languages is the tesseract -l argument. Defaults to "eng".
	Languages string
	// Timeout bounds a single OCR run. Defaults to 2 minutes.
	Timeout time.Duration
}

func (t *Tesseract) binary() string {
	if t.Binary != "" {
		return t.Binary
	}
	return "tesseract"
}

// Available reports whether the tesseract binary can be found.
func (t *Tesseract) Available() bool {
	_, err := exec.LookPath(t.binary())
	return err == nil
}

// Extract runs tesseract on the given image and returns the recognized text.
func (t *Tesseract) Extract(ctx context.Context, path string) (string, error) {
	timeout := t.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	langs := t.Languages
	if langs == "" {
		langs = "eng"
	}

	cmd := exec.CommandContext(ctx, t.binary(), path, "stdout", "-l", langs)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("ocr timed out after %s", timeout)
		}
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("tesseract failed: %s", msg)
	}
	return strings.TrimSpace(out.String()), nil
}
