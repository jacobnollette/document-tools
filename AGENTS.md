# Repository Guide

document-tools is a self-hosted document management system: photos of receipts
and other documents (plus PDFs and markdown) are uploaded from a phone, their
text extracted server-side, stored in PostgreSQL, and browsed through a web
interface with universal-format downloads. A standalone document conversion
CLI (the project's original tool) lives alongside it.

## Layout

- `cmd/server` — HTTP server: JSON API under `/api` plus the built web app
- `cmd/document-tools` — conversion CLI wrapping `pandoc`/`pdftotext`
- `internal/config` — first-run installer config (`config.json` in the data dir)
- `internal/db` — Postgres connection + embedded SQL migrations
- `internal/store` — metadata/OCR text in Postgres, original files + page previews on disk
- `internal/auth` — users (bcrypt) and HMAC-signed session cookies
- `internal/ocr` — processing engine (tesseract, pdftotext/pdftoppm) + async worker pool
- `internal/convert` — download renditions: markdown/text/searchable PDF (tesseract, pandoc+typst)
- `internal/api` — HTTP handlers; `setup.go` is the pre-install wizard API
- `internal/testdb` — per-package Postgres test databases (skipped without `TEST_DATABASE_URL`)
- `web/` — React + TypeScript + Vite single-page app
- `deploy/terraform/` — home-lab stack: app + Postgres containers on a Docker host
- `.github/workflows/` — `ci.yml` (every push/PR) and `deploy.yml` (image publish to GHCR)

## Conventions

- **Lifecycle**: the server boots into setup mode until `config.json` exists;
  the installer swaps in the full handler without a restart. Document status
  flows `pending → processing → completed | failed` via `store.UpdateStatus`.
- **Auth**: single implicit permission level — accounts have no role field;
  never add one casually. All `/api/documents*` routes require a session.
- **Uploads**: content type is sniffed from file bytes, never trusted from the
  client. Allowed: images, PDF, markdown/plain text. Document IDs and stored
  filenames are server-generated — user-supplied names must never influence paths.
- **Storage split**: metadata and extracted text belong in Postgres; bytes
  (originals, previews) belong on disk under the data dir. Keep it that way so
  the filesystem can live on a shared mount.
- **Schema changes** are new numbered files in `internal/db/migrations/` —
  never edit an applied migration.
- **External tools** (tesseract, poppler, pandoc, typst) are shelled out to
  behind interfaces (`ocr.Engine`) with fakes in tests; keep handlers free of
  exec calls.
- **Frontend**: keep dependencies minimal (react, react-router-dom). The build
  must pass `tsc --noEmit` (strict mode) — `npm run build` enforces it.

## Checks to run before pushing

```bash
gofmt -l .            # must print nothing
go vet ./...
# database tests need Postgres:
#   docker run -d -e POSTGRES_USER=test -e POSTGRES_PASSWORD=test -p 15432:5432 postgres:16-alpine
TEST_DATABASE_URL=postgres://test:test@localhost:15432/test?sslmode=disable go test ./...
cd web && npm run build
terraform -chdir=deploy/terraform fmt -check   # if terraform is installed
```

CI runs all of the above plus a Docker image build with an HTTP smoke test.

## Local development

- API: `go run ./cmd/server` (install `tesseract-ocr` + `poppler-utils`, and
  `pandoc` + `typst` for PDF downloads)
- Web: `cd web && npm run dev` (proxies `/api` to `localhost:8080`)
- Full stack: see the Docker instructions in `README.md`

## Deployment status

Images publish to GHCR from `main`. The Terraform stack targets a home-lab
Docker host and is applied manually from a machine that can reach it — see
`deploy/terraform/terraform.tfvars.example`. Nothing applies automatically.
