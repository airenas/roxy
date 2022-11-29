--SQL statements for a status table
BEGIN;

CREATE TABLE status(
    id UUID NOT NULL PRIMARY KEY,
    status TEXT NOT NULL,
    status_external TEXT,
    error_code TEXT,
    error TEXT,
    audio_ready BOOL DEFAULT FALSE,
    available_results JSONB,
    created TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMIT;