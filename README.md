# document-tools (macOS)

Minimal, focused command‑line helpers for working with documents on macOS. This repo will gradually assemble a small set of pragmatic tools. First up: two Pandoc‑style functions for markdown to pdf, and pdf to markdown.

## What’s included (initial tools)

- Installation script (macOS/Homebrew) that sets up:
  - pandoc (Markdown > PDF)
  - poppler (provides PDF > Markdown)
  - BasicTeX (minimal LaTeX needed by Pandoc to make PDFs)
- Two Bash functions that mimic Pandoc’s CLI style:
  - pdf2md: Wraps pdftotext -layout and supports an optional -o output flag
  - md2pdf: Wraps pandoc input -o output to generate PDFs

The goal is a tiny, opinionated toolkit—do the common thing well, avoid endless flags and preferences.

## Requirements

- macOS (Terminal users; zsh or bash)
- Homebrew installed
- Internet access to install formulae/casks

The install script uses Homebrew to install:
- pandoc
- poppler (for pdftotext)
- basictex (LaTeX engine Pandoc uses for PDF output)

Note: Some PDFs may require additional LaTeX packages. BasicTeX is minimal; Pandoc might prompt you to install missing packages as needed.

## Usage

- Convert PDF → Markdown (default output: input.md)
```
pdf2md input.pdf
```

- Convert PDF → Markdown with -o
```
pdf2md input.pdf -o output.md
```

- Convert Markdown → PDF (default output: input.pdf)
```
md2pdf input.md
```

- Convert Markdown → PDF with -o
```
md2pdf input.md -o output.pdf
```

## How it works

- pdf2md:
  - Runs pdftotext -layout input.pdf - to preserve text column/layout spacing
  - Pipes to sed to strip trailing whitespace
  - Redirects to output .md file

- md2pdf:
  - Calls pandoc input.md -o output.pdf
  - Pandoc uses a LaTeX engine (provided by BasicTeX) to produce the PDF

## Limitations / Notes

- PDF → Markdown is best‑effort. Complex layouts (multi‑column, tables, figures) may need manual cleanup.
- The helpers intentionally keep the interface simple: 1 input plus optional -o output. Other Pandoc/pdftotext flags are not passed through by default.
- If Pandoc PDF generation complains about missing LaTeX packages, install them via tlmgr or a fuller TeX distribution.
- If the installer fails on pandoc, verify Homebrew is installed and try:
  - brew install pandoc
  - brew install poppler
  - brew install --cask basictex
