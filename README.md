# document-tools

A self-hosted document management system. Upload receipts and other documents
from your phone, let the server extract their text with OCR, and browse,
search-ready, from a web interface.

The pipeline: **upload (photo, PDF, or markdown) → text extraction → stored in
PostgreSQL, browsable and downloadable in a universal format**.

## Architecture

| Component | Location | Stack |
| --- | --- | --- |
| API server | `cmd/server`, `internal/` | Go, PostgreSQL, Tesseract + poppler + pandoc |
| Web app | `web/` | React + TypeScript + Vite |
| Conversion CLI | `cmd/document-tools` | Go wrapper around pandoc/pdftotext |
| Infrastructure | `deploy/terraform/` | Terraform → home-lab Docker host |
| Pipelines | `.github/workflows/` | CI on every push/PR; image publish to GHCR |

- **Originals stay originals**: uploaded files are kept byte-for-byte on the
  local filesystem (`DATA_DIR`, typically a shared-storage mount), one
  directory per document alongside rendered page previews.
- **Text lives in the database**: extracted text and metadata go into
  PostgreSQL (with a full-text search index ready for a search feature).
- **Universal formats on the way out**: any document can be downloaded as the
  original, Markdown, plain text, or PDF — searchable PDF via Tesseract for
  images, pandoc/typst for markdown.
- **Processing** is async: `pending → processing → completed | failed`, with
  interrupted documents re-queued on startup. Images are OCRed with Tesseract;
  PDFs use their embedded text layer when present and are OCRed page by page
  when scanned.

## First-run installer

The app installs itself WordPress-style. On first launch it serves a setup
wizard that asks for PostgreSQL connection details and creates the first user
account (every account has full access — there are no roles). `DB_HOST`,
`DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, and `DB_SSLMODE` environment
variables pre-fill the form; everything stays editable in the browser. The
installer connects, creates the schema, saves `config.json` into the data
directory, and logs you in.

### API

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/setup/status` | Install state (+ env-derived defaults pre-install) |
| `POST` | `/api/setup` | Run the installer (pre-install only) |
| `POST` | `/api/auth/login` / `logout` | Session management |
| `GET` | `/api/auth/me` | Current user |
| `POST` | `/api/documents` | Upload (multipart `file`, 50 MB max) |
| `GET` | `/api/documents` | List, newest first |
| `GET` | `/api/documents/{id}` | Metadata + extracted text |
| `GET` | `/api/documents/{id}/file` | Original file |
| `GET` | `/api/documents/{id}/pages/{n}` | Rendered page preview |
| `GET` | `/api/documents/{id}/download?format=` | `original`, `markdown`, `text`, or `pdf` |
| `DELETE` | `/api/documents/{id}` | Remove a document |

All document routes require a session.

## Running locally

### With Docker

```bash
docker build -t document-tools .
docker run -d --name documents-db -e POSTGRES_USER=documents \
  -e POSTGRES_PASSWORD=secret -e POSTGRES_DB=documents postgres:16-alpine
docker run -p 8080:8080 --link documents-db:db \
  -e DB_HOST=db -e DB_USER=documents -e DB_PASSWORD=secret \
  -v document-tools-data:/data document-tools
```

Open http://localhost:8080 and complete the installer (the DB fields arrive
pre-filled from the environment).

### For development

Backend (needs `tesseract-ocr`, `poppler-utils`, and optionally
`pandoc` + `typst` on PATH for full functionality):

```bash
go run ./cmd/server        # API on :8080, serves the setup wizard first
```

Frontend with hot reload (proxies `/api` to :8080):

```bash
cd web
npm install
npm run dev                # UI on :5173
```

Tests (database tests need Postgres):

```bash
docker run -d -e POSTGRES_USER=test -e POSTGRES_PASSWORD=test -p 15432:5432 postgres:16-alpine
export TEST_DATABASE_URL=postgres://test:test@localhost:15432/test?sslmode=disable
go test ./...
cd web && npm run build    # type-checks and builds
```

## Deployment (home lab)

Images are published to GHCR by the `Build & Publish` workflow on every push
to `main`. The Terraform stack in `deploy/terraform/` deploys the app and
PostgreSQL to a Docker host, with all persistent state under a single
`data_root` directory — point it at your shared-storage mount (Ceph, NFS,
SMB, or a plain disk):

```bash
cd deploy/terraform
cp terraform.tfvars.example terraform.tfvars   # edit data_root, db_password, ...
terraform init
terraform apply
```

Run the apply from any machine that can reach the Docker host (set
`docker_host = "ssh://user@host"` for remote applies).

## Conversion CLI

The original standalone document conversion tool still lives at
`cmd/document-tools` and works as before:

```bash
go build -o document-tools ./cmd/document-tools
document-tools -i notes.md -o notes.pdf
document-tools -i report.pdf -o report.md
```

It requires `pandoc` (and `pdftotext` from poppler for PDF input).

## Project intent

- This is a personal hobby project.
- It makes no claim of commercial advantage or profit over related projects/tools.
- If you are a maintainer/owner of a related project and would like this taken down or changed, please reach out and I will address it promptly.
