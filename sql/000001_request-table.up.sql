--SQL statements for a request table
BEGIN;

--Schema for requests table
CREATE TABLE requests(
    id uuid not null,
    created timestamp default current_timestamp,
    updated timestamp default current_timestamp
);

GRANT ALL ON ALL TABLES IN SCHEMA public TO roxy;

COMMIT;
