-- FindName returns a column named like its parameter, which the generated function must not
-- declare twice.
-- name: FindName :one
select name from things where name = ?;

-- name: FindNames :many
select name from things where name like ?;

-- name: FindKinds :many
select kind from things where maybe_kind = sqlc.narg(maybe_kind);

-- name: FindPayload :one
select payload from things where id = ?;

-- name: CountLarge :one
select count(*) from things where kind = 'large' and rating > ?;
