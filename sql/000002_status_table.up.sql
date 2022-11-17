--SQL statements for a request table
BEGIN;

--Schema for requests table
CREATE TABLE status(
    id uuid not null,
    status TEXT NOT NULL,
    created timestamp default current_timestamp,
    updated timestamp default current_timestamp
);

COMMIT;
