--SQL statements for a status table
BEGIN;

CREATE TABLE status(
    id TEXT NOT NULL PRIMARY KEY,
    status TEXT NOT NULL,
    status_external TEXT,
    progress INT DEFAULT 0,
    error_code TEXT,
    error TEXT,
    audio_ready BOOL DEFAULT FALSE,
    available_results JSONB,
    created TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    version INT DEFAULT 0
);

COMMIT;