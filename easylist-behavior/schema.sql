-- Schema of the behaviour snapshot fixture, read by lists/lists.go. Every
-- object is created unqualified, in the current schema: each test run creates
-- a fresh schema and puts it first on search_path. plan and roles are
-- PostgreSQL enums, as an application would declare them.

CREATE TYPE person_plan AS ENUM ('free', 'pro', 'team');
CREATE TYPE person_role AS ENUM ('admin', 'editor', 'viewer');

CREATE TABLE countries (
	id integer PRIMARY KEY,
	name text NOT NULL,
	code text NOT NULL
);

CREATE TABLE people (
	id bigint PRIMARY KEY,
	uid uuid NOT NULL UNIQUE,
	name text NOT NULL,
	nickname text,
	age integer,
	height double precision,
	balance numeric,
	active boolean,
	plan person_plan NOT NULL,
	due date,
	last_seen timestamptz,
	created_at timestamptz NOT NULL,
	country_id integer REFERENCES countries (id),
	deleted_at timestamptz,
	tags text[],
	lucky_numbers bigint[],
	ratings double precision[],
	prices numeric[],
	answers boolean[],
	holidays date[],
	logins timestamptz[],
	friend_ids uuid[],
	roles person_role[] NOT NULL
);

CREATE TABLE orders (
	id bigint PRIMARY KEY,
	person_id bigint NOT NULL REFERENCES people (id),
	placed_at timestamptz NOT NULL
);
