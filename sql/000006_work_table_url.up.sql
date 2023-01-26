--adding work table fields
BEGIN;

ALTER TABLE
    work_data
ADD
    COLUMN transcriber_url TEXT;

ALTER TABLE
    work_data
ADD
    COLUMN try_count INT DEFAULT 0;

ALTER TABLE
    work_data
ADD
    COLUMN version INT DEFAULT 0;

ALTER TABLE
    work_data
ADD
    COLUMN updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP;

COMMIT;