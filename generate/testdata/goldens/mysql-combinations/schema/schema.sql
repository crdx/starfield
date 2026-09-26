create table resources (
    id bigint signed not null auto_increment primary key,
    created_at datetime not null,
    deleted_at datetime default null,
    api_url varchar(255) not null,
    payload binary(16) not null,
    optional_payload blob default null,
    ratio float not null,
    enabled boolean not null,
    category enum('alpha', 'beta') not null
);

create table audit_entries (
    id bigint signed not null auto_increment primary key,
    occurred_on date not null,
    duration time default null,
    year_value year not null,
    description text not null
);

create table metadata (
    code char(4) not null,
    value text default null
);

create view resources_view as
select id, api_url, enabled from resources where deleted_at is null;
