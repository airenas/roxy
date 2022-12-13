--SQL statements for a work table
BEGIN;

CREATE TABLE work_data(
    id TEXT NOT NULL PRIMARY KEY,
    external_ID TEXT NOT NULL,
    created TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMIT;