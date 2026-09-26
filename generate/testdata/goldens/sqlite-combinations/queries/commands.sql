-- name: ListDocuments :many
select * from documents where deleted_at is null order by id;

-- name: FindDocumentBody :one
select body from documents where id = ?;

-- name: FindDocumentsByIDs :many
select * from documents
where id in (sqlc.slice('ids'))
order by id;

-- name: UpdateDocument :execrows
update documents
set body = sqlc.arg(body), rank = sqlc.arg(rank), active = sqlc.arg(active)
where id = sqlc.arg(id);

-- name: DeleteCounter :exec
delete from counters where name = ?;

-- name: IncrementCounter :execresult
update counters set value = value + 1 where name = ?;
