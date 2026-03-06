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

## Build from source (with Go installed)

If you already have Go installed, you can build locally:

```bash
git clone https://github.com/<your-org-or-user>/document-tools.git
cd document-tools
go build -o document-tools .
./document-tools -h
```

Or install directly to your Go bin path:

```bash
go install .
```

Then ensure your Go bin directory is on `PATH` (commonly `$(go env GOPATH)/bin` or `~/go/bin`).

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

- This project does **not** publish signed binaries from GitHub Actions.
- I currently do not have a code-signing certificate available in this repo environment, so builds/signing are not done on GitHub.
- For now, build locally if you want to run from source.

## Project Intent

- This is a personal hobby project.
- It makes no claim of commercial advantage or profit over related projects/tools.
- If you are a maintainer/owner of a related project and would like this taken down or changed, please reach out and I will address it promptly.
