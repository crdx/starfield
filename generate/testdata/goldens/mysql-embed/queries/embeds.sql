-- name: ListBooksWithAuthors :many
select sqlc.embed(books), sqlc.embed(authors)
from books
join authors on authors.id = books.author_id
order by books.id;

-- name: FindBookWithAuthorName :one
select sqlc.embed(books), authors.name as author_name
from books
join authors on authors.id = books.author_id
where books.id = ?;

-- name: ListBooksByAuthorName :many
select sqlc.embed(books)
from books
join authors on authors.id = books.author_id
where authors.name = ?;

-- name: ListCoauthorPairs :many
select sqlc.embed(first), sqlc.embed(second)
from authors first
join authors second on second.id > first.id;


-- name: ListReviewsWithBooks :many
select sqlc.embed(book_reviews), sqlc.embed(books)
from book_reviews
join books on books.id = book_reviews.book_id;
