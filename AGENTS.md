# Repository Guide

document-tools is a self-hosted document management system: photos of receipts
and other documents are uploaded from a phone, OCRed server-side, and browsed
through a web interface. A standalone document conversion CLI (the project's
original tool) lives alongside it.

## Layout

- `cmd/server` — HTTP server: JSON API under `/api` plus the built web app
- `cmd/document-tools` — conversion CLI wrapping `pandoc`/`pdftotext`
- `internal/store` — filesystem-backed document store (metadata, original file, OCR text per document directory)
- `internal/ocr` — OCR engine interface, Tesseract implementation, async worker pool
- `internal/api` — HTTP handlers and routing
- `web/` — React + TypeScript + Vite single-page app
- `deploy/terraform/` — AWS stack: ECR, ECS Fargate, EFS, ALB (not yet applied)
- `.github/workflows/` — `ci.yml` (every push/PR) and `deploy.yml` (manual)

## Conventions

- **Backend**: Go standard library only — no third-party modules. New storage
  or OCR backends should implement the existing interfaces (`ocr.Engine`,
  or extend `store`) rather than leaking into handlers.
- **Document lifecycle**: `pending → processing → completed | failed`. Status
  transitions go through `store.UpdateStatus` so `ProcessedAt`/`Error` stay
  consistent.
- **Uploads**: content type is sniffed from file bytes, not trusted from the
  client; only image types are accepted. Document IDs and stored filenames are
  server-generated — user-supplied names must never influence paths.
- **Frontend**: keep dependencies minimal (react, react-router-dom). The build
  must pass `tsc --noEmit` (strict mode) — `npm run build` enforces it.
- **CLI behavior contract**: quoted paths and spaces supported, outputs
  overwritten, non-zero exit on failure, consistent error formatting.

## Checks to run before pushing

```bash
gofmt -l .            # must print nothing
go vet ./...
go test ./...
cd web && npm run build
terraform -chdir=deploy/terraform fmt -check -recursive   # if terraform is installed
```

CI runs all of the above plus a Docker image build with an HTTP smoke test.

## Local development

- API: `go run ./cmd/server` (uses `./data`; install `tesseract-ocr` for working OCR)
- Web: `cd web && npm run dev` (proxies `/api` to `localhost:8080`)
- Full stack in one container: `docker build -t document-tools . && docker run -p 8080:8080 document-tools`

## Deployment status

Nothing is deployed yet. The Terraform stack validates in CI but has never
been applied; the `Deploy` workflow is `workflow_dispatch`-gated and its
one-time setup steps (state backend, OIDC role, repo variables) are documented
in the workflow file header.
