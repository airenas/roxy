--SQL statements for a request table
BEGIN;

--Schema for requests table
CREATE TABLE requests(
    id TEXT NOT NULL PRIMARY KEY,
    email TEXT,
    file_count INT DEFAULT 0,
    file_name TEXT,
    file_names JSONB,
    params JSONB,
    request_id TEXT NOT NULL,
    created TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_requests_created_selector ON requests (created);

COMMIT;