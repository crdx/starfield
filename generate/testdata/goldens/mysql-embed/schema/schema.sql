create table authors (
    id int unsigned not null auto_increment primary key,
    name varchar(255) not null,
    born_on date default null
);

create table books (
    id int unsigned not null auto_increment primary key,
    author_id int unsigned not null,
    title varchar(255) not null,
    created_at datetime not null
);

create table book_reviews (
    id int unsigned not null auto_increment primary key,
    book_id int unsigned not null,
    rating tinyint unsigned not null
);
