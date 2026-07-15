import { useCallback, useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import {
  deleteDocument,
  documentDownloadUrl,
  documentPageUrl,
  getDocument,
  listDocuments,
  type Doc,
  type DownloadFormat,
} from "../api";
import StatusBadge from "../components/StatusBadge";

const POLL_MS = 2000;

const downloadOptions: { format: DownloadFormat; label: string }[] = [
  { format: "original", label: "Original file" },
  { format: "markdown", label: "Markdown (.md)" },
  { format: "text", label: "Plain text (.txt)" },
  { format: "pdf", label: "PDF (searchable)" },
];

export default function DocumentDetailPage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const [doc, setDoc] = useState<Doc | null>(null);
  const [neighbors, setNeighbors] = useState<{ prev?: string; next?: string }>({});
  const [page, setPage] = useState(1);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      setDoc(await getDocument(id));
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load document");
    }
  }, [id]);

  useEffect(() => {
    setPage(1);
    void refresh();
  }, [refresh]);

  // Neighbors for prev/next navigation across the library (list is newest
  // first, so "previous" is the newer document).
  useEffect(() => {
    listDocuments()
      .then((docs) => {
        const i = docs.findIndex((d) => d.id === id);
        setNeighbors({
          prev: i > 0 ? docs[i - 1].id : undefined,
          next: i >= 0 && i < docs.length - 1 ? docs[i + 1].id : undefined,
        });
      })
      .catch(() => {});
  }, [id]);

  const active = doc?.status === "pending" || doc?.status === "processing";
  useEffect(() => {
    if (!active) return;
    const timer = setInterval(() => void refresh(), POLL_MS);
    return () => clearInterval(timer);
  }, [active, refresh]);

  // Arrow keys page through the document; with the doc fully paged, they
  // move between documents.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (!doc) return;
      if (e.key === "ArrowRight") {
        if (page < Math.max(doc.page_count, 1)) setPage(page + 1);
        else if (neighbors.next) navigate(`/documents/${encodeURIComponent(neighbors.next)}`);
      } else if (e.key === "ArrowLeft") {
        if (page > 1) setPage(page - 1);
        else if (neighbors.prev) navigate(`/documents/${encodeURIComponent(neighbors.prev)}`);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [doc, page, neighbors, navigate]);

  async function handleDelete() {
    if (!window.confirm("Delete this document permanently?")) return;
    await deleteDocument(id);
    navigate("/");
  }

  function handleDownload(format: string) {
    if (!format) return;
    window.location.href = documentDownloadUrl(id, format as DownloadFormat);
  }

  if (error) {
    return (
      <div>
        <p className="error-text">{error}</p>
        <Link to="/">← Back to documents</Link>
      </div>
    );
  }
  if (!doc) return <p>Loading…</p>;

  const pageCount = Math.max(doc.page_count, 1);
  const isText = doc.content_type.startsWith("text/");

  return (
    <div>
      <div className="toolbar">
        <Link to="/">← All documents</Link>
        <div className="toolbar-actions">
          <select
            className="download-select"
            value=""
            onChange={(e) => handleDownload(e.target.value)}
            aria-label="Download as"
          >
            <option value="" disabled>
              ⬇ Download…
            </option>
            {downloadOptions.map((o) => (
              <option key={o.format} value={o.format}>
                {o.label}
              </option>
            ))}
          </select>
          <button className="btn btn-danger" onClick={() => void handleDelete()}>
            Delete
          </button>
        </div>
      </div>

      <h1 className="detail-title">{doc.original_filename}</h1>
      <p className="doc-date">
        Uploaded {new Date(doc.uploaded_at).toLocaleString()} ·{" "}
        {(doc.size_bytes / 1024).toFixed(0)} KB · <StatusBadge status={doc.status} />
      </p>

      {doc.status === "failed" && doc.error && (
        <p className="error-text">Text extraction failed: {doc.error}</p>
      )}
      {active && <p className="hint-text">Processing… this page updates automatically.</p>}

      <div className="doc-nav">
        {neighbors.prev ? (
          <Link to={`/documents/${encodeURIComponent(neighbors.prev)}`}>← Newer</Link>
        ) : (
          <span />
        )}
        {pageCount > 1 && (
          <span className="pager">
            <button
              className="btn btn-ghost"
              disabled={page <= 1}
              onClick={() => setPage(page - 1)}
            >
              ‹
            </button>
            Page {page} of {pageCount}
            <button
              className="btn btn-ghost"
              disabled={page >= pageCount}
              onClick={() => setPage(page + 1)}
            >
              ›
            </button>
          </span>
        )}
        {neighbors.next ? (
          <Link to={`/documents/${encodeURIComponent(neighbors.next)}`}>Older →</Link>
        ) : (
          <span />
        )}
      </div>

      <div className="detail-grid">
        {!isText && (
          <div className="detail-image">
            <img
              src={documentPageUrl(doc.id, page)}
              alt={`${doc.original_filename} page ${page}`}
            />
          </div>
        )}
        <div className="detail-text">
          <h2>Extracted text</h2>
          {doc.status === "completed" ? (
            doc.ocr_text ? (
              <pre>{doc.ocr_text}</pre>
            ) : (
              <p className="hint-text">No text was found in this document.</p>
            )
          ) : (
            <p className="hint-text">Text will appear here once processing finishes.</p>
          )}
        </div>
      </div>
    </div>
  );
}
