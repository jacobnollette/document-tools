# document-tools (macOS)

Minimal, opinionated document conversion CLI.

## Install

1. Download `document-tools` binary from GitHub Releases.
2. Move it into your PATH (example):

```bash
mv document-tools /usr/local/bin/document-tools
chmod +x /usr/local/bin/document-tools
```

3. Install runtime dependencies (macOS/Homebrew):

```bash
brew install pandoc poppler
brew install --cask basictex
```

## Usage

```bash
document-tools -i input.ext -o output.ext
```

- `-i` optional: if omitted, reads stdin as Markdown
- `-o` required: output format is inferred from extension
- output files are overwritten

Examples:

```bash
document-tools -i notes.md -o notes.pdf
document-tools -i report.pdf -o report.md
cat notes.md | document-tools -o notes.docx
```

## Notes

- PDF input is extracted with `pdftotext -layout` (from `poppler`) and then converted.
- PDF → Markdown is best effort; complex layouts may need cleanup.
- Pandoc PDF output depends on LaTeX packages (BasicTeX may require extra packages).

## CI / Release

- GitHub Actions builds macOS binaries on PRs and pushes to `main`.
- Pushing a tag like `v2.0.0` publishes build artifacts to GitHub Releases.
