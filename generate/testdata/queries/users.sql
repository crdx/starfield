-- name: FetchActiveUsers :many
select * from users where active = 1 and deleted_at is null;

-- name: FetchUserByEmail :one
select * from users where email = ?;

-- name: CountUsers :one
select count(*) from users;

-- name: DeactivateUser :exec
update users set active = 0 where id = ?;
