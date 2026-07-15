// Package ocr extracts text from uploaded documents and runs the async
// processing queue that moves documents from pending to completed.
package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Result is what processing a document produces.
type Result struct {
	// Text is the extracted (OCR or native) text.
	Text string
	// PageCount is the number of pages/previews the document has.
	PageCount int
}

// Engine turns an uploaded file into extracted text plus page previews.
type Engine interface {
	// Process extracts text from the file at path. For multi-page formats it
	// also renders per-page PNG previews into previewDir (page-0001.png, ...).
	Process(ctx context.Context, path, contentType, previewDir string) (Result, error)
}

// Pipeline is the production Engine. It shells out to tesseract for image
// OCR and poppler (pdftotext/pdftoppm) for PDF handling.
type Pipeline struct {
	// Languages is the tesseract -l argument. Defaults to "eng".
	Languages string
	// Timeout bounds a single external command. Defaults to 2 minutes.
	Timeout time.Duration
}

// MissingTools returns the names of required external binaries that cannot be
// found on PATH, so startup can warn about an incomplete environment.
func (p *Pipeline) MissingTools() []string {
	var missing []string
	for _, tool := range []string{"tesseract", "pdftotext", "pdftoppm"} {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	return missing
}

func (p *Pipeline) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%s timed out after %s", name, timeout)
		}
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s failed: %s", name, msg)
	}
	return out.Bytes(), nil
}

func (p *Pipeline) languages() string {
	if p.Languages != "" {
		return p.Languages
	}
	return "eng"
}

// Process dispatches on content type.
func (p *Pipeline) Process(ctx context.Context, path, contentType, previewDir string) (Result, error) {
	switch {
	case contentType == "application/pdf":
		return p.processPDF(ctx, path, previewDir)
	case strings.HasPrefix(contentType, "image/"):
		text, err := p.ocrImage(ctx, path)
		if err != nil {
			return Result{}, err
		}
		return Result{Text: text, PageCount: 1}, nil
	case strings.HasPrefix(contentType, "text/"):
		data, err := os.ReadFile(path)
		if err != nil {
			return Result{}, fmt.Errorf("read text document: %w", err)
		}
		return Result{Text: string(data), PageCount: 1}, nil
	default:
		return Result{}, fmt.Errorf("unsupported content type %q", contentType)
	}
}

func (p *Pipeline) ocrImage(ctx context.Context, path string) (string, error) {
	out, err := p.run(ctx, "tesseract", path, "stdout", "-l", p.languages())
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// processPDF renders page previews, then extracts embedded text — falling
// back to OCR of the rendered pages for scanned PDFs with no text layer.
func (p *Pipeline) processPDF(ctx context.Context, path, previewDir string) (Result, error) {
	pages, err := p.renderPDFPages(ctx, path, previewDir)
	if err != nil {
		return Result{}, err
	}

	out, err := p.run(ctx, "pdftotext", "-layout", path, "-")
	if err != nil {
		return Result{}, err
	}
	text := strings.TrimSpace(string(out))

	if text == "" {
		// No embedded text layer — OCR the rendered pages instead.
		var parts []string
		for _, page := range pages {
			pageText, err := p.ocrImage(ctx, page)
			if err != nil {
				return Result{}, err
			}
			parts = append(parts, pageText)
		}
		text = strings.TrimSpace(strings.Join(parts, "\n\n"))
	}
	return Result{Text: text, PageCount: len(pages)}, nil
}

// renderPDFPages rasterizes each PDF page to previewDir/page-NNNN.png and
// returns the ordered file paths.
func (p *Pipeline) renderPDFPages(ctx context.Context, path, previewDir string) ([]string, error) {
	if err := os.MkdirAll(previewDir, 0o755); err != nil {
		return nil, fmt.Errorf("create preview directory: %w", err)
	}
	if _, err := p.run(ctx, "pdftoppm", "-png", "-r", "150", path, filepath.Join(previewDir, "raw")); err != nil {
		return nil, err
	}

	rendered, err := filepath.Glob(filepath.Join(previewDir, "raw-*.png"))
	if err != nil || len(rendered) == 0 {
		return nil, errors.New("pdf produced no page previews")
	}
	// pdftoppm zero-pads page numbers to a consistent width per run, so a
	// lexical sort is page order.
	sort.Strings(rendered)

	pages := make([]string, len(rendered))
	for i, src := range rendered {
		dst := filepath.Join(previewDir, fmt.Sprintf("page-%04d.png", i+1))
		if err := os.Rename(src, dst); err != nil {
			return nil, fmt.Errorf("finalize preview page: %w", err)
		}
		pages[i] = dst
	}
	return pages, nil
}
