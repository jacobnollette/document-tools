CREATE TABLE users (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username      text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE documents (
    id                text PRIMARY KEY,
    original_filename text NOT NULL,
    content_type      text NOT NULL,
    size_bytes        bigint NOT NULL,
    status            text NOT NULL,
    error             text NOT NULL DEFAULT '',
    ocr_text          text NOT NULL DEFAULT '',
    page_count        integer NOT NULL DEFAULT 0,
    uploaded_at       timestamptz NOT NULL,
    processed_at      timestamptz
);

CREATE INDEX documents_uploaded_at_idx ON documents (uploaded_at DESC);

-- Full-text search over extracted text, ready for a future search feature.
CREATE INDEX documents_ocr_text_fts_idx ON documents
    USING gin (to_tsvector('english', ocr_text));
