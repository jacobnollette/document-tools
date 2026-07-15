import { useCallback, useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { deleteDocument, documentFileUrl, getDocument, type Doc } from "../api";
import StatusBadge from "../components/StatusBadge";

const POLL_MS = 2000;

export default function DocumentDetailPage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const [doc, setDoc] = useState<Doc | null>(null);
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
    void refresh();
  }, [refresh]);

  const active = doc?.status === "pending" || doc?.status === "processing";
  useEffect(() => {
    if (!active) return;
    const timer = setInterval(() => void refresh(), POLL_MS);
    return () => clearInterval(timer);
  }, [active, refresh]);

  async function handleDelete() {
    if (!window.confirm("Delete this document permanently?")) return;
    await deleteDocument(id);
    navigate("/");
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

  return (
    <div>
      <div className="toolbar">
        <Link to="/">← Back</Link>
        <button className="btn btn-danger" onClick={() => void handleDelete()}>
          Delete
        </button>
      </div>

      <h1 className="detail-title">{doc.original_filename}</h1>
      <p className="doc-date">
        Uploaded {new Date(doc.uploaded_at).toLocaleString()} ·{" "}
        {(doc.size_bytes / 1024).toFixed(0)} KB · <StatusBadge status={doc.status} />
      </p>

      {doc.status === "failed" && doc.error && (
        <p className="error-text">Text extraction failed: {doc.error}</p>
      )}
      {active && <p className="hint-text">Extracting text… this page updates automatically.</p>}

      <div className="detail-grid">
        <div className="detail-image">
          <img src={documentFileUrl(doc.id)} alt={doc.original_filename} />
        </div>
        <div className="detail-text">
          <h2>Extracted text</h2>
          {doc.status === "completed" ? (
            doc.ocr_text ? (
              <pre>{doc.ocr_text}</pre>
            ) : (
              <p className="hint-text">No text was found in this image.</p>
            )
          ) : (
            <p className="hint-text">Text will appear here once processing finishes.</p>
          )}
        </div>
      </div>
    </div>
  );
}
