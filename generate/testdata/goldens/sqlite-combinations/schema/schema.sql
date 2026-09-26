create table documents (
    id integer not null primary key,
    created_at datetime not null,
    deleted_at datetime,
    body text not null,
    payload blob,
    rank real not null,
    active boolean not null
);

create table counters (
    name text not null primary key,
    value integer not null
);

create view document_summaries_view as
select id, body, active from documents where deleted_at is null;
