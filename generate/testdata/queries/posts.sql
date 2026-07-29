-- name: FetchPostsByUser :many
select * from posts where user_id = ? order by created_at desc;

-- name: FetchPublishedPostTitles :many
select title from posts where published = 1;
