# document-tools (macOS)

Minimal, focused document conversion helpers for macOS.

This project is moving from shell aliases to a single Go CLI binary: `document-tools`.

## What’s included

- Installation script (macOS/Homebrew) that sets up:
  - `pandoc` (format conversions)
  - `poppler` (`pdftotext` for PDF input)
  - BasicTeX (minimal LaTeX needed by Pandoc to make PDFs)
- A Go CLI: `document-tools`
- Legacy bash aliases (`pdf2md`, `md2pdf`) remain in the repo during transition

The goal is a tiny, opinionated toolkit: do the common thing well, avoid endless flags and preferences.

## Requirements

- macOS (Terminal users; zsh or bash)
- Homebrew installed
- Internet access to install formulae/casks

The install script uses Homebrew to install:
- pandoc
- poppler (for pdftotext)
- basictex (LaTeX engine Pandoc uses for PDF output)

Note: Some PDFs may require additional LaTeX packages. BasicTeX is minimal; Pandoc might prompt you to install missing packages as needed.

## Usage (Go CLI)

General shape:

```bash
document-tools -i input.ext -o output.ext
```

Examples:

- Markdown file → PDF
```bash
document-tools -i notes.md -o notes.pdf
```

- Markdown file → HTML
```bash
document-tools -i notes.md -o notes.html
```

- PDF file → Markdown (uses `pdftotext -layout` first)
```bash
document-tools -i report.pdf -o report.md
```

- stdin Markdown → DOCX
```bash
cat notes.md | document-tools -o notes.docx
```

Notes:
- If `-i` is omitted, input is read from stdin and treated as Markdown.
- Output format is inferred from the `-o` file extension.
- If input is PDF, `pdftotext` is used to extract text before conversion.
- Output files are overwritten by default.

## How it works

- Non-PDF input: read file (or stdin) and pass through `pandoc` with explicit `-f` and `-t`
- PDF input: run `pdftotext -layout input.pdf -` first, then send extracted text to `pandoc`
- Markdown target from Markdown source is written directly without pandoc

## Limitations / Notes

- PDF → Markdown is best-effort. Complex layouts (multi-column, tables, figures) may need manual cleanup.
- The CLI intentionally keeps the interface small: optional `-i` plus required `-o`.
- If Pandoc PDF generation complains about missing LaTeX packages, install them via tlmgr or a fuller TeX distribution.
- If the installer fails on pandoc, verify Homebrew is installed and try:
  - brew install pandoc
  - brew install poppler
  - brew install --cask basictex
