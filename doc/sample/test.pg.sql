DROP TABLE IF EXISTS users CASCADE;
DROP TABLE IF EXISTS articles CASCADE;
DROP TABLE IF EXISTS tags CASCADE;
DROP TABLE IF EXISTS article_tags CASCADE;


CREATE TABLE users (
    id bigserial UNIQUE NOT NULL,
    nick_name varchar(128) NOT NULL,
    password varchar(128),
    profile text,

    PRIMARY KEY (id)
);

CREATE TABLE articles (
    id bigserial UNIQUE NOT NULL,
    title varchar(256) NOT NULL,
    contents text NOT NULL,
    owner_user_id bigint NOT NULL,

    PRIMARY KEY (id)
);

CREATE TABLE tags (
    id bigserial UNIQUE NOT NULL,
    name varchar(256) UNIQUE NOT NULL,

    PRIMARY KEY (id)
);

CREATE TABLE article_tags (
    id bigserial UNIQUE NOT NULL,
    article_id bigint NOT NULL,
    tag_id bigint NOT NULL,

    PRIMARY KEY (id)
);

