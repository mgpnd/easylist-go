package conformance

import easylist "github.com/mgpnd/easylist-go/easylist-core"

// users is the one list every case plans against. Its fields cover every
// type and every array element type, author SQL with ?, $, #, and :
// characters, and joins, one of which also holds a ?. name is not declared
// nullable although one row holds NULL, as in the "NULL comparison on a
// non-nullable field" scenario. Cases may replace the scope.
var users = easylist.List{
	Name: "users", Table: "users", Alias: "u", PrimaryKey: "id",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "name", Type: easylist.Text},
		{Name: "age", Type: easylist.Integer, Nullable: true},
		{Name: "score", Type: easylist.Float, Nullable: true},
		{Name: "price", Type: easylist.Numeric, Nullable: true},
		{Name: "active", Type: easylist.Boolean, Nullable: true},
		{Name: "uid", Type: easylist.UUID, Nullable: true},
		{Name: "status", Type: easylist.Enum, Nullable: true, Values: []string{"success", "pending", "error"}},
		{Name: "due", Type: easylist.Date, Nullable: true},
		{Name: "created_at", Type: easylist.Timestamp, Nullable: true},
		{Name: "country", Type: easylist.Text, Nullable: true, SQL: "c.name", Joins: []string{"country"}},
		{Name: "code", Type: easylist.Text, Nullable: true, SQL: `regexp_replace(u.code, '^#\$', '')`},
		{Name: "label", Type: easylist.Text, Nullable: true, SQL: "CASE WHEN u.meta ? 'vip' THEN 'vip:' || u.name ELSE u.name END"},
		{Name: "tier", Type: easylist.Text, Nullable: true, SQL: "m.tier", Joins: []string{"meta"}},
		{Name: "vip", Type: easylist.Boolean, Nullable: true, SQL: "m.vip", Joins: []string{"meta"}},
		{Name: "tags", Type: easylist.Text, Array: true, Nullable: true},
		{Name: "ids", Type: easylist.Integer, Array: true, Nullable: true},
		{Name: "statuses", Type: easylist.Enum, Array: true, Nullable: true, Values: []string{"ok", "passed", "retried", "skipped", "failed"}},
		{Name: "prices", Type: easylist.Numeric, Array: true, Nullable: true},
		{Name: "days", Type: easylist.Date, Array: true, Nullable: true},
		{Name: "seen", Type: easylist.Timestamp, Array: true, Nullable: true},
		{Name: "local_seen", Type: easylist.Timestamp, Array: true, Nullable: true},
	},
	Joins: []easylist.Join{
		{Key: "country", SQL: "LEFT JOIN countries c ON c.id = u.country_id"},
		{Key: "meta", SQL: "LEFT JOIN LATERAL (SELECT u.meta ->> 'tier' AS tier, u.meta ? 'vip' AS vip) m ON TRUE"},
	},
}

// fixtureDDL creates the fixture in the current schema, the first entry of
// the connection's search_path. status and statuses are PostgreSQL enums,
// so IN and = ANY must compare with them without a cast.
const fixtureDDL = `
CREATE TYPE status_t AS ENUM ('success', 'pending', 'error');
CREATE TYPE run_status AS ENUM ('ok', 'passed', 'retried', 'skipped', 'failed');
CREATE TABLE countries (id integer PRIMARY KEY, name text NOT NULL);
CREATE TABLE users (
	id bigint PRIMARY KEY,
	name text,
	age integer,
	score double precision,
	price numeric,
	active boolean,
	uid uuid,
	status status_t,
	due date,
	created_at timestamptz,
	country_id integer REFERENCES countries (id),
	deleted_at timestamptz,
	meta jsonb,
	code text,
	tags text[],
	ids int8[],
	statuses run_status[],
	prices numeric[],
	days date[],
	seen timestamptz[],
	local_seen timestamp[]
);
`

// fixtureRows are the seed rows. Every case's expected ids are written by
// hand against this table:
//
//	id name              age score price    active status  due        created_at (UTC)            country deleted meta
//	 1 Ann                25  1.5  9.50     true   success 2026-11-02 2026-10-26 21:59:59         FR      -       {vip,tier gold}
//	 2 Ann                35  2.5  1234...10 false  pending 2026-11-01 2026-10-26 22:00:00         France  yes     {tier silver}
//	 3 a                  30 -1.25 2.5      NULL   error   2026-01-01 2026-10-27 09:30:00.123456  Spain   -       {vip false}
//	 4 b                  40  NULL 2.50     true   NULL    2026-02-01 2026-10-27 09:30:00.123457  NULL    yes     NULL
//	 5 Mega 50%_OFF sale  41  3    1.01     false  success 2026-02-02 2026-10-27 21:59:59         FR      -       {}
//	 6 500 off            29  NULL 9.51     true   pending 2000-01-02 2026-10-27 22:00:00         NULL    -       NULL
//	 7 C:\temp            NULL NULL 1234...11 NULL NULL    NULL       2026-11-01 23:59:59         Spain   -       NULL
//	 8 NULL               NULL ...                                    2026-11-02 00:00:00         NULL
//	 9 Bob                30  NULL NULL     true   NULL    NULL       2026-10-24 21:59:59
//	10 Cy                                                             2026-10-24 22:00:00
//	11 Dee                                                            2026-10-25 22:30:00
//	12 Eve                                                            2026-10-25 23:00:00
//	13 x'; DROP TABLE...                                              2026-01-01 00:00:00
//	14 Fay                                                            2026-02-01 00:00:00
//	15 Gus                                                            2026-02-01 00:00:00.000001
//	16 50%_off                                                        2025-12-31 23:59:59
//	17 Hal                                                            NULL
//
// 1234...10 and 1234...11 are 12345678901234567890.10 and .11, which a
// float64 cannot tell apart. Rows 1 to 7 also hold the code, uid, and array
// values below; every other array is NULL.
const fixtureRows = `
INSERT INTO countries (id, name) VALUES (1, 'FR'), (2, 'France'), (3, 'Spain');
INSERT INTO users (id, name, age, score, price, active, uid, status, due, created_at, country_id, deleted_at, meta, code,
                   tags, ids, statuses, prices, days, seen, local_seen) VALUES
(1, 'Ann', 25, 1.5, 9.50, true, '7f1c0d2e-0000-4000-8000-000000000000', 'success', '2026-11-02', '2026-10-26 21:59:59+00', 1, NULL,
 '{"vip": true, "tier": "gold"}', '#$x',
 '{go,rust}', '{7,8}', '{ok,passed,retried}', '{10.50,12345678901234567890.10}', '{2026-10-25,2026-10-26}',
 '{"2026-10-27 10:00:00+00"}', '{"2026-10-27 10:00:00"}'),
(2, 'Ann', 35, 2.5, 12345678901234567890.10, false, '0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b', 'pending', '2026-11-01', '2026-10-26 22:00:00+00', 2, '2026-01-01 00:00:00+00',
 '{"tier": "silver"}', '$x',
 '{golang,Go}', '{7}', '{ok}', '{}', NULL, '{"2026-10-27 08:00:00+00"}', NULL),
(3, 'a', 30, -1.25, 2.5, NULL, NULL, 'error', '2026-01-01', '2026-10-27 09:30:00.123456+00', 3, NULL,
 '{"vip": false}', '#x',
 '{}', '{}', '{ok,skipped}', NULL, NULL, NULL, NULL),
(4, 'b', 40, NULL, 2.50, true, NULL, NULL, '2026-02-01', '2026-10-27 09:30:00.123457+00', NULL, '2026-05-01 00:00:00+00',
 NULL, NULL,
 NULL, NULL, '{skipped,failed}', NULL, NULL, NULL, NULL),
(5, 'Mega 50%_OFF sale', 41, 3, 1.01, false, NULL, 'success', '2026-02-02', '2026-10-27 21:59:59+00', 1, NULL,
 '{}', NULL,
 '{a,b}', '{8,9}', '{}', NULL, NULL, NULL, NULL),
(6, '500 off', 29, NULL, 9.51, true, NULL, 'pending', '2000-01-02', '2026-10-27 22:00:00+00', NULL, NULL,
 NULL, NULL,
 '{"a,b"," c"}', NULL, NULL, NULL, NULL, NULL, NULL),
(7, 'C:\temp', NULL, NULL, 12345678901234567890.11, NULL, NULL, NULL, NULL, '2026-11-01 23:59:59+00', 3, NULL,
 NULL, NULL,
 '{go}', NULL, NULL, NULL, NULL, NULL, NULL);
INSERT INTO users (id, name, age, active, created_at) VALUES
(8, NULL, NULL, NULL, '2026-11-02 00:00:00+00'),
(9, 'Bob', 30, true, '2026-10-24 21:59:59+00'),
(10, 'Cy', NULL, NULL, '2026-10-24 22:00:00+00'),
(11, 'Dee', NULL, NULL, '2026-10-25 22:30:00+00'),
(12, 'Eve', NULL, NULL, '2026-10-25 23:00:00+00'),
(13, 'x''; DROP TABLE users; --', NULL, NULL, '2026-01-01 00:00:00+00'),
(14, 'Fay', NULL, NULL, '2026-02-01 00:00:00+00'),
(15, 'Gus', NULL, NULL, '2026-02-01 00:00:00.000001+00'),
(16, '50%_off', NULL, NULL, '2025-12-31 23:59:59+00'),
(17, 'Hal', NULL, NULL, NULL);
`
