import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { listDocuments, documentFileUrl, type Doc } from "../api";
import StatusBadge from "../components/StatusBadge";
import UploadButton from "../components/UploadButton";

const POLL_MS = 3000;

export default function DocumentListPage() {
  const [docs, setDocs] = useState<Doc[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      setDocs(await listDocuments());
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load documents");
    } finally {
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  // Keep polling while anything is still moving through the OCR pipeline.
  const hasActive = docs.some(
    (d) => d.status === "pending" || d.status === "processing",
  );
  useEffect(() => {
    if (!hasActive) return;
    const timer = setInterval(() => void refresh(), POLL_MS);
    return () => clearInterval(timer);
  }, [hasActive, refresh]);

  return (
    <div>
      <div className="toolbar">
        <h1>Documents</h1>
        <UploadButton onUploaded={() => void refresh()} />
      </div>

      {error && <p className="error-text">{error}</p>}

      {loaded && docs.length === 0 && !error && (
        <div className="empty-state">
          <p>No documents yet.</p>
          <p>Upload a receipt photo to get started — it will be OCRed automatically.</p>
        </div>
      )}

      <ul className="doc-grid">
        {docs.map((doc) => (
          <li key={doc.id} className="doc-card">
            <Link to={`/documents/${encodeURIComponent(doc.id)}`}>
              <div className="doc-thumb">
                <img src={documentFileUrl(doc.id)} alt={doc.original_filename} loading="lazy" />
              </div>
              <div className="doc-meta">
                <span className="doc-name">{doc.original_filename}</span>
                <span className="doc-date">
                  {new Date(doc.uploaded_at).toLocaleString()}
                </span>
                <StatusBadge status={doc.status} />
              </div>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
