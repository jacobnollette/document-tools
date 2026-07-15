import { useRef, useState } from "react";
import { uploadDocument, type Doc } from "../api";

interface Props {
  onUploaded: (doc: Doc) => void;
}

// Mobile-friendly upload: accept="image/*" lets phones offer the camera or
// photo library directly from the file picker.
export default function UploadButton({ onUploaded }: Props) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleFiles(files: FileList | null) {
    if (!files || files.length === 0) return;
    setBusy(true);
    setError(null);
    try {
      for (const file of Array.from(files)) {
        const doc = await uploadDocument(file);
        onUploaded(doc);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "upload failed");
    } finally {
      setBusy(false);
      if (inputRef.current) inputRef.current.value = "";
    }
  }

  return (
    <div className="upload">
      <input
        ref={inputRef}
        type="file"
        accept="image/*"
        multiple
        hidden
        onChange={(e) => handleFiles(e.target.files)}
      />
      <button
        className="btn btn-primary"
        disabled={busy}
        onClick={() => inputRef.current?.click()}
      >
        {busy ? "Uploading…" : "＋ Upload receipt"}
      </button>
      {error && <p className="error-text">{error}</p>}
    </div>
  );
}
