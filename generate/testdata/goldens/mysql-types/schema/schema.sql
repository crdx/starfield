create table things (
    id int unsigned not null auto_increment primary key,
    name varchar(255) not null,
    flag tinyint(1) not null,
    maybe_flag boolean null,
    rating tinyint unsigned not null,
    offset_value tinyint null,
    payload json not null,
    maybe_payload json null,
    kind enum('small', 'large') not null,
    maybe_kind enum('red', 'green') null
);
