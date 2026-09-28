create table users (
    id int unsigned not null auto_increment primary key,
    created_at datetime not null,
    deleted_at datetime default null,
    name varchar(255) not null,
    email varchar(255) default null,
    age int default null,
    active boolean not null,
    balance decimal(10, 2) not null,
    score double default null,
    notes text default null,
    avatar blob default null,
    last_login datetime default null
);

create table posts (
    id int unsigned not null auto_increment primary key,
    created_at datetime not null,
    user_id int unsigned not null,
    title varchar(255) not null,
    body text not null,
    published boolean not null
);
