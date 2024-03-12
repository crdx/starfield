create table foos (
    id int not null auto_increment primary key,
    created_at datetime not null,
    deleted_at datetime default null,
    name text not null
);
