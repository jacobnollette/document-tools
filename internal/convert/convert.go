// Package convert produces download formats for a document: its extracted
// text as Markdown or plain text, and a PDF rendition — searchable PDF via
// tesseract for images, pandoc for text documents, or the original when the
// upload already is a PDF.
package convert

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Format is a downloadable rendition of a document.
type Format string

const (
	FormatOriginal Format = "original"
	FormatMarkdown Format = "markdown"
	FormatText     Format = "text"
	FormatPDF      Format = "pdf"
)

// ParseFormat validates a user-supplied format string.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatOriginal, FormatMarkdown, FormatText, FormatPDF, "":
		if s == "" {
			return FormatOriginal, nil
		}
		return Format(s), nil
	default:
		return "", fmt.Errorf("unknown format %q (expected original, markdown, text, or pdf)", s)
	}
}

// Converter renders documents into download formats. Timeout bounds each
// external command; it defaults to 2 minutes.
type Converter struct {
	Timeout time.Duration
}

// run executes an external command with dir as its working directory —
// pandoc and typst create temp files relative to the cwd, so it must be
// writable.
func (c *Converter) run(ctx context.Context, dir, name string, args ...string) error {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var errOut bytes.Buffer
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%s timed out after %s", name, timeout)
		}
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s failed: %s", name, msg)
	}
	return nil
}

// ToPDF renders the document at inputPath (with the given content type and
// extracted text) into a PDF file and returns its path. The caller owns the
// returned file's parent directory cleanup via the returned cleanup func.
func (c *Converter) ToPDF(ctx context.Context, inputPath, contentType, extractedText string) (string, func(), error) {
	if contentType == "application/pdf" {
		return inputPath, func() {}, nil
	}

	tmpDir, err := os.MkdirTemp("", "dt-convert-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp directory: %w", err)
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	switch {
	case strings.HasPrefix(contentType, "image/"):
		// tesseract's pdf output embeds the image with an invisible text
		// layer — a searchable PDF of the original scan.
		outBase := filepath.Join(tmpDir, "out")
		if err := c.run(ctx, tmpDir, "tesseract", inputPath, outBase, "pdf"); err != nil {
			cleanup()
			return "", nil, err
		}
		return outBase + ".pdf", cleanup, nil

	case strings.HasPrefix(contentType, "text/"):
		outPath := filepath.Join(tmpDir, "out.pdf")
		if err := c.run(ctx, tmpDir, "pandoc", "-f", "markdown", "--pdf-engine=typst",
			"-o", outPath, inputPath); err != nil {
			cleanup()
			return "", nil, err
		}
		return outPath, cleanup, nil

	default:
		// Fall back to a text-based PDF from whatever was extracted.
		srcPath := filepath.Join(tmpDir, "extracted.md")
		if err := os.WriteFile(srcPath, []byte(extractedText), 0o600); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("write extracted text: %w", err)
		}
		outPath := filepath.Join(tmpDir, "out.pdf")
		if err := c.run(ctx, tmpDir, "pandoc", "-f", "markdown", "--pdf-engine=typst",
			"-o", outPath, srcPath); err != nil {
			cleanup()
			return "", nil, err
		}
		return outPath, cleanup, nil
	}
}

// BaseName strips the extension from an uploaded filename for use in
// Content-Disposition names.
func BaseName(filename string) string {
	base := filepath.Base(filename)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
