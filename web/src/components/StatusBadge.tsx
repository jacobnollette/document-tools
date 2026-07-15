import type { DocumentStatus } from "../api";

const labels: Record<DocumentStatus, string> = {
  pending: "Queued",
  processing: "Processing…",
  completed: "Ready",
  failed: "Failed",
};

export default function StatusBadge({ status }: { status: DocumentStatus }) {
  return <span className={`badge badge-${status}`}>{labels[status]}</span>;
}
