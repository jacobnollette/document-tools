# Agents (Go CLI) Reference

This project is moving from shell aliases to a **single Go binary** that wraps macOS document/file tools behind a unified interface.

Goal: one predictable CLI for common file conversion and inspection tasks, while keeping each command minimal and opinionated.

See `README.md` for install/setup details.

## Direction

- Primary interface: one Go CLI executable (for example: `document-tools ...`)
- Scope: wrap existing best-of-breed tools (`pandoc`, `pdftotext`, etc.) instead of re-implementing them
- Philosophy: simple defaults, minimal flags, consistent behavior across commands

## Prerequisites (macOS)

- Homebrew
- `pandoc`
- `poppler` (for `pdftotext`)
- A LaTeX engine for PDF generation (BasicTeX is sufficient for most cases)

## Current Command Agents

These are the first two agents and should remain supported as subcommands in the Go CLI.

### `pdf2md`

- Synopsis: `pdf2md input.pdf [-o output.md]`
- Behavior: runs `pdftotext -layout` and writes markdown-friendly text output
- Default output: if `-o` is omitted, output is `input.md`

Examples:
```
pdf2md invoice.pdf
pdf2md report.pdf -o report.md
```

### `md2pdf`

- Synopsis: `md2pdf input.md [-o output.pdf]`
- Behavior: runs `pandoc input.md -o output.pdf` (via LaTeX)
- Default output: if `-o` is omitted, output is `input.pdf`

Examples:
```
md2pdf notes.md
md2pdf notes.md -o notes.pdf
```

## Unified CLI Behavior Contract

For all agents/subcommands in the Go app:

- Support quoted paths and filenames with spaces
- Overwrite output if it already exists (unless a future command explicitly documents otherwise)
- Use consistent error formatting
- Return non-zero exit code on failure
- Keep interfaces small: one input + optional `-o` unless there is a clear need for more

## Notes & Limitations

- PDF → Markdown remains best-effort; complex layouts may require manual cleanup
- Pandoc PDF output depends on LaTeX packages; BasicTeX may need extra package installs

## Troubleshooting

- Command not found:
  - Ensure the Go binary is installed and on `PATH`
  - During transition, if using legacy aliases, ensure `document-tools-alias.sh` is sourced

- `pdf2md` output is empty/incorrect:
  - Verify `pdftotext -v`
  - Inspect raw output with `pdftotext -layout input.pdf -`
  - Scanned PDFs may require OCR before conversion

- `md2pdf` fails with LaTeX errors:
  - Install missing packages via `tlmgr` or use a fuller TeX distribution
  - Verify `pandoc --version`

## Extending the Go Agents

When adding new wrappers, prefer:

- A focused command for one real workflow
- Shared I/O and error handling in common Go utilities
- Avoiding pass-through flag explosions; add options only when repeatedly needed
