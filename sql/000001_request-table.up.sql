--SQL statements for a request table
BEGIN;

--Schema for requests table
CREATE TABLE requests(
    id uuid not null,
    created timestamp default current_timestamp,
    updated timestamp default current_timestamp
);

COMMIT;
