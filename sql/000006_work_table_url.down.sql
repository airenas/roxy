BEGIN;

ALTER TABLE
    work_data DROP COLUMN version;

ALTER TABLE
    work_data DROP COLUMN try_count;

ALTER TABLE
    work_data DROP COLUMN transcriber_url;

COMMIT;