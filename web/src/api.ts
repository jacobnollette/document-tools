export type DocumentStatus = "pending" | "processing" | "completed" | "failed";

export interface Doc {
  id: string;
  original_filename: string;
  content_type: string;
  size_bytes: number;
  status: DocumentStatus;
  error?: string;
  page_count: number;
  uploaded_at: string;
  processed_at?: string;
  ocr_text?: string;
}

export interface User {
  id: number;
  username: string;
}

export interface DatabaseSettings {
  host: string;
  port: string;
  user: string;
  password: string;
  name: string;
  sslmode: string;
}

export interface SetupStatus {
  installed: boolean;
  defaults?: DatabaseSettings;
}

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

async function asError(res: Response): Promise<ApiError> {
  try {
    const body = await res.json();
    if (body && typeof body.error === "string")
      return new ApiError(body.error, res.status);
  } catch {
    // fall through to the generic message
  }
  return new ApiError(`request failed (${res.status})`, res.status);
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (!res.ok) throw await asError(res);
  return res.json();
}

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw await asError(res);
  return res.json();
}

// --- setup & auth ---

export function getSetupStatus(): Promise<SetupStatus> {
  return get("/api/setup/status");
}

export function install(
  database: DatabaseSettings,
  admin: { username: string; password: string },
): Promise<{ installed: boolean; user: User }> {
  return postJSON("/api/setup", { database, admin });
}

export function login(username: string, password: string): Promise<User> {
  return postJSON("/api/auth/login", { username, password });
}

export async function logout(): Promise<void> {
  await fetch("/api/auth/logout", { method: "POST" });
}

export function me(): Promise<User> {
  return get("/api/auth/me");
}

// --- documents ---

export async function listDocuments(): Promise<Doc[]> {
  const body = await get<{ documents: Doc[] }>("/api/documents");
  return body.documents ?? [];
}

export function getDocument(id: string): Promise<Doc> {
  return get(`/api/documents/${encodeURIComponent(id)}`);
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

export function documentPageUrl(id: string, page: number): string {
  return `/api/documents/${encodeURIComponent(id)}/pages/${page}`;
}

export type DownloadFormat = "original" | "markdown" | "text" | "pdf";

export function documentDownloadUrl(id: string, format: DownloadFormat): string {
  return `/api/documents/${encodeURIComponent(id)}/download?format=${format}`;
}
