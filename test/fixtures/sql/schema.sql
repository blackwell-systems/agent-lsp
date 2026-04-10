CREATE TABLE person (
  id   SERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  age  INTEGER      NOT NULL
);

CREATE TABLE post (
  id        SERIAL PRIMARY KEY,
  title     VARCHAR(200) NOT NULL,
  author_id INTEGER      NOT NULL REFERENCES person(id)
);
