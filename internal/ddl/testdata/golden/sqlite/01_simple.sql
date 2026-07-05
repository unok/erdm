DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS posts;


CREATE TABLE users (
    id int NOT NULL,
    name varchar(64) NOT NULL,

    PRIMARY KEY (id)
);

CREATE TABLE posts (
    id int NOT NULL,
    user_id int NOT NULL,

    PRIMARY KEY (id)
);

