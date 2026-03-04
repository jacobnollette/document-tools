# Agents (CLI helpers) Reference

This repository exposes two tiny command “agents” for the terminal. They wrap common PDF/Markdown conversions with a simple, Pandoc‑style interface.

See README.md for installation and environment setup.

## Prerequisites

- macOS with Homebrew
- poppler (for `pdftotext`)
- pandoc
- A LaTeX engine (BasicTeX is sufficient for most cases)

## Commands

### pdf2md

- Synopsis: `pdf2md input.pdf [-o output.md]`
- What it does: Uses `pdftotext -layout` to extract text, preserving spacing to approximate the original layout, then strips trailing whitespace and writes Markdown‑friendly text to a file.
- Defaults: If `-o` is omitted, output becomes `input.md` (same basename as the input).

Examples:
```
pdf2md invoice.pdf
pdf2md report.pdf -o report.md
```

### md2pdf

- Synopsis: `md2pdf input.md [-o output.pdf]`
- What it does: Uses `pandoc input.md -o output.pdf` to render a PDF via LaTeX.
- Defaults: If `-o` is omitted, output becomes `input.pdf` (same basename as the input).

Examples:
```
md2pdf notes.md
md2pdf notes.md -o notes.pdf
```

## Behavior and Notes

- Filenames with spaces are supported (the functions quote paths).
- If the output file exists, it will be overwritten.
- PDF → Markdown is best‑effort; complex layouts (tables, multi‑column, figures) may need manual cleanup.
- Pandoc PDF output relies on LaTeX. BasicTeX is minimal; you may need to install extra LaTeX packages if Pandoc requests them.

## Troubleshooting

- “command not found: pdf2md/md2pdf”
  - Ensure your shell has sourced `document-tools-alias.sh` (see README.md). Open a new terminal or `source ~/.zshrc` / `~/.bashrc`.

- pdf2md produces an empty/incorrect file
  - Verify `pdftotext -v` works.
  - Try `pdftotext -layout input.pdf -` to inspect raw output. Some PDFs (scanned images) need OCR first.

- md2pdf fails with LaTeX errors
  - Install missing LaTeX packages (tlmgr) or use a fuller TeX distribution.
  - Check `pandoc --version` to confirm it’s installed.

## Extending the helpers

The wrappers are intentionally minimal. If you need more control:

- Add default Pandoc flags (template, variables, PDF engine) in `md2pdf`.
- Forward additional arguments by enhancing the basic `-o` parsing.
- Post‑process `pdf2md` output (e.g., normalize headings or lists) by adjusting the `sed` step.
