export type DocumentStatus = "pending" | "processing" | "completed" | "failed";

export interface Doc {
  id: string;
  original_filename: string;
  content_type: string;
  size_bytes: number;
  status: DocumentStatus;
  error?: string;
  uploaded_at: string;
  processed_at?: string;
  ocr_text?: string;
}

async function asError(res: Response): Promise<Error> {
  try {
    const body = await res.json();
    if (body && typeof body.error === "string") return new Error(body.error);
  } catch {
    // fall through to the generic message
  }
  return new Error(`request failed (${res.status})`);
}

export async function listDocuments(): Promise<Doc[]> {
  const res = await fetch("/api/documents");
  if (!res.ok) throw await asError(res);
  const body = await res.json();
  return body.documents ?? [];
}

export async function getDocument(id: string): Promise<Doc> {
  const res = await fetch(`/api/documents/${encodeURIComponent(id)}`);
  if (!res.ok) throw await asError(res);
  return res.json();
}

export async function uploadDocument(file: File): Promise<Doc> {
  const form = new FormData();
  form.append("file", file);
  const res = await fetch("/api/documents", { method: "POST", body: form });
  if (!res.ok) throw await asError(res);
  return res.json();
}

export async function deleteDocument(id: string): Promise<void> {
  const res = await fetch(`/api/documents/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
  if (!res.ok && res.status !== 404) throw await asError(res);
}

export function documentFileUrl(id: string): string {
  return `/api/documents/${encodeURIComponent(id)}/file`;
}
