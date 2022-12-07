--SQL statements for a email_lock table
BEGIN;

CREATE TABLE email_lock(
    id UUID NOT NULL,
    key TEXT NOT NULL,
    value INT DEFAULT 0,
    created TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_email_lock_selector ON email_lock (id, key);

COMMIT;