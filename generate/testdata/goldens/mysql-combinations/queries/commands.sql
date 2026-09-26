-- name: ListResources :many
select * from resources where deleted_at is null order by id;

-- name: ListResourceURLs :many
select api_url from resources where deleted_at is null order by id;

-- name: FindResourceSummary :one
select id, api_url from resources where id = ?;

-- name: CountResources :one
select count(*) from resources where deleted_at is null;

-- name: SearchResources :many
select * from resources
where enabled = ? and category = ? and api_url like ?
order by id;

-- name: FindResourcesByIDs :many
select * from resources
where id in (sqlc.slice('ids'))
order by id;

-- name: UpdateResource :execrows
update resources
set api_url = sqlc.arg(api_url), enabled = sqlc.arg(enabled)
where id = sqlc.arg(id);

-- name: DisableResource :exec
update resources set enabled = false where id = ?;

-- name: InsertAuditEntry :execlastid
insert into audit_entries (occurred_on, duration, year_value, description)
values (?, ?, ?, ?);

-- name: TouchAuditEntry :execresult
update audit_entries set description = ? where id = ?;

-- name: Type :one
select api_url as err from resources where id = ?;
