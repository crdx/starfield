-- name: FetchFoos :many
select * from foos where deleted_at is null;
