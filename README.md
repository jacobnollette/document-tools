# document-tools

A self-hosted document management system. Upload receipts and other documents
from your phone, let the server extract their text with OCR, and browse
everything from a web interface.

The MVP pipeline: **photo upload → OCR (Tesseract) → searchable text in the web UI**.

## Architecture

| Component | Location | Stack |
| --- | --- | --- |
| API server | `cmd/server`, `internal/` | Go (stdlib only), Tesseract for OCR |
| Web app | `web/` | React + TypeScript + Vite |
| Conversion CLI | `cmd/document-tools` | Go wrapper around pandoc/pdftotext |
| Infrastructure | `deploy/terraform/` | AWS: ECR, ECS Fargate, EFS, ALB |
| Pipelines | `.github/workflows/` | CI on every push/PR, manual deploy |

Documents are stored on the filesystem (`DATA_DIR`) — one directory per
document holding the original image, extracted text, and metadata. OCR runs
asynchronously in an in-process worker pool; documents move through
`pending → processing → completed | failed` and the UI polls until they settle.

### API

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/api/documents` | Upload an image (multipart field `file`, 25 MB max) |
| `GET` | `/api/documents` | List documents, newest first |
| `GET` | `/api/documents/{id}` | Metadata plus extracted text once completed |
| `GET` | `/api/documents/{id}/file` | The original image |
| `DELETE` | `/api/documents/{id}` | Remove a document |
| `GET` | `/api/healthz` | Health check |

## Running locally

### With Docker (includes Tesseract)

```bash
docker build -t document-tools .
docker run -p 8080:8080 -v document-tools-data:/data document-tools
```

Open http://localhost:8080.

### For development

Backend (install `tesseract-ocr` locally for working OCR):

```bash
go run ./cmd/server        # API on :8080
```

Frontend with hot reload (proxies `/api` to :8080):

```bash
cd web
npm install
npm run dev                # UI on :5173
```

Tests:

```bash
go test ./...
cd web && npm run build    # type-checks and builds
```

## Deployment

The Terraform stack in `deploy/terraform/` provisions an ECR repository, an
ECS Fargate service with an EFS volume for document storage, and an ALB. It
has **not been applied yet** — the `Deploy` workflow is manual-trigger only.
Setup steps (state backend, OIDC role, repository variables) are documented at
the top of [`.github/workflows/deploy.yml`](.github/workflows/deploy.yml).

CI runs on every push and pull request: Go format/vet/test, web type-check and
build, a container image build with an HTTP smoke test, and
`terraform fmt`/`validate`.

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
