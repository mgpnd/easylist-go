# easylist-sqlpg

`easylist-sqlpg` (package `sqlpg`) is the shared core of easylist's PostgreSQL drivers. It is for the authors of `easylist-sql-*` drivers. Applications never import it; they pick the driver for the query library they already use, as listed in [easylist-core's README](../easylist-core/README.md#drivers).

The module has two parts:

- Package `sqlpg` lowers an [easylist-core](../easylist-core) plan into a typed description of one PostgreSQL query. It makes every decision that depends on the plan, so a driver contains no type, operator, or NULL logic.
- Package [`conformance`](#run-the-conformance-suite) is the test suite every PostgreSQL driver runs. It checks arguments and errors without a database and, when `EASYLIST_PG_DSN` is set, which rows each filter matches on a real PostgreSQL.

## Local development

`easylist-filter` and `easylist-core` are not published yet, so this module builds only inside the [easylist-go](https://github.com/mgpnd/easylist-go) repository, whose root `go.work` links the modules. Clone that repository and work from it.

## From plan to query

```
easylist.Plan --sqlpg.Lower--> *sqlpg.Query --driver maps--> (sql, args)
```

A driver's `Compile` calls `sqlpg.Lower` and maps the returned `Query` onto its library:

```go
func (*Driver) Compile(p *easylist.Plan) (string, []any, error) {
	q, err := sqlpg.Lower(p)
	if err != nil {
		return "", nil, err // already the contract's error text
	}
	// Map q.From, q.Select, q.Where, q.Order, and q.Page onto the library.
}
```

`Lower` returns every validation error with the text all PostgreSQL drivers share: unknown fields, unsupported operators, literals that do not convert, invalid lists, and missing evaluated values. A driver adds errors only for failures inside its library, and wraps them with the list name.

The `Query` holds:

| Field | Contents |
| --- | --- |
| `Count` | Set for a count query: `SELECT COUNT(*)` with no `Select`, `Order`, or `Page`. |
| `From` | The table, `AS`, the alias, and the planned joins in order, as one verbatim `Expr`. |
| `Select` | One `Column` per field in plan order: its `Expr` and its alias, the field name, to write as a quoted identifier. Array fields are already wrapped in `to_json(...)::text`. |
| `Where` | A `Cond` tree, or nil. The list's scope is a `Raw` node ANDed in front of the filter. |
| `Order` | `Ordering` entries: an `Expr` and `Desc`. |
| `Page` | The limit and offset of a page query, or nil. Both are always bound, including an offset of 0. |

`Query.Args()` returns the arguments the driver must return, with their Go types: the filter values left to right, then limit and offset as `int64`. The conformance suite compares every driver's arguments with it.

### Conditions

`Cond` is one of `And`, `Or`, `Raw`, `Compare`, `IsNull`, `In`, `Between`, `ILike`, `ArrayHas`, or `DayRange`. Every node's `Fragment()` returns its canonical PostgreSQL template: `SQL` with one `?` per slot, and `Slots`, each an `Expr` to write verbatim or an `Arg` to bind. For example, `In{Not: true, Null: NullMatch}` with two values gives `COALESCE(? NOT IN (?, ?), TRUE)` with the slots column, first value, second value. An `Arg` with `Cast` `"numeric"` already has `::numeric` after its `?`, and no other value is ever cast.

A driver walks the tree. It maps `And` and `Or` onto its library's connectors, and every other node either onto a native construct that gives the same semantics with no cast and no extra argument, or onto its library's raw-with-arguments primitive over `Fragment()`. A driver that renders only fragments, parenthesizing `And` and `Or` as their fragments do, produces the SQL of `easylist-sql-goqu`. The fragment is the reference: a driver's golden tests compare each native mapping with it.

The semantics behind the nodes, such as `IS DISTINCT FROM` for `!=`, the NULL forms of `IN`, and whole-day ranges, are listed under [PostgreSQL semantics](#postgresql-semantics).

### Author SQL is an opaque leaf

Every library rewrites some syntax inside the SQL it scans: `?` for goqu literals with arguments, bob's `psql.Raw`, and gorm's `clause.Expr`; `$` for go-sqlbuilder; named-argument tokens for jet's `Raw`. List-author SQL may contain any of them, as in a `u.meta ? 'vip'` scope or a `regexp_replace(u.code, '^#\$', '')` field. So:

- A driver never concatenates an `Expr` into a template that its library then scans.
- Each `Expr` is written through a construct that copies it without scanning, such as `goqu.L(sql)` with no arguments, bob's `expr.Raw(sql)`, or ent's `b.WriteString(sql)`.
- A fragment's `SQL` never contains author SQL, which is always a slot, so it is safe to hand to a scanning primitive with the slots as its arguments.

The conformance suite runs author SQL containing `?`, `$`, `#`, and `:` through every driver.

## Query shape

Every PostgreSQL driver produces queries of this shape, with these semantics, because they come from `Lower`. The examples show the canonical fragments.

- Each selected field is returned under its field name. A field without SQL reads `alias."name"`; a field with SQL emits it verbatim (and in parentheses inside `WHERE`).
- `FROM` is the table, `AS`, the alias, then the planned joins in order, all verbatim.
- The list's scope is wrapped in parentheses and ANDed with the filter, so an `OR` in the filter cannot escape it.
- Page queries always bind `LIMIT` and `OFFSET`, after the filter arguments. Export queries have neither. Count queries are `SELECT COUNT(*)` with only the joins the filter needs, and no ordering or pagination.
- Placeholders are `$1`, `$2`, ... in the order the filter is read, left to right. Compiling the same plan twice produces the same SQL and arguments. The SQL text may differ between drivers in whitespace and layout; the arguments, their Go types, the errors, and the matching rows do not.

List-author SQL must not contain `$n` placeholders of its own, because they would collide with the bound arguments. Other placeholder-like characters (`?`, `$` not followed by a digit, `#`, `:`) are written verbatim and bind nothing.

## PostgreSQL semantics

- `!=` compiles to `IS DISTINCT FROM`, so it matches exactly the rows `=` does not, including rows whose field is NULL. To skip NULL rows as `<>` did, add `AND x != NULL`, as in `country != 'FR' AND country != NULL`.
- `= NULL` compiles to `IS NULL` and `!= NULL` to `IS NOT NULL`, with no bound argument. Every type accepts them, whether or not the field is declared `Nullable`.
- `IN` and `NOT IN` bind each non-NULL item once, in list order, inside an SQL list, so PostgreSQL infers each argument's type from the column as it does for `=`, and an enum list needs no cast. A `NULL` item is never bound or placed in the SQL list. With `L` the bound items, such as `$1, $2`, the field's SQL is emitted once:

  | Filter | SQL |
  | --- | --- |
  | `x IN (L)` | `x IN (L)` |
  | `x IN (L, NULL)` | `COALESCE(x IN (L), TRUE)` |
  | `x NOT IN (L)` | `COALESCE(x NOT IN (L), TRUE)` |
  | `x NOT IN (L, NULL)` | `COALESCE(x NOT IN (L), FALSE)` |
  | `x IN (NULL)` | `x IS NULL` |
  | `x NOT IN (NULL)` | `x IS NOT NULL` |

  A `NULL` item may be anywhere in the list, and a list of only `NULL` items compiles like `(NULL)`. So a NULL field matches `IN` only when the list holds `NULL`, and `NOT IN` only when it does not. This differs from writing `NULL` inside an SQL `NOT IN` list, which matches no rows at all. Numeric items keep their `::numeric` cast, as in `x IN ($1::numeric, $2::numeric)`.
- `CONTAINS` on text compiles to `ILIKE '%…%' ESCAPE E'\\'`: case-insensitive, with `%`, `_`, and `\` in the literal matched literally.
- `CONTAINS` and `EXCLUDES` on text reject a parenthesized list with an error naming the field at the `(`.
- `EXCLUDES` on text compiles to `COALESCE(x NOT ILIKE '%…%' ESCAPE E'\\', TRUE)`, with the same escaping as `CONTAINS`. It matches rows that do not contain the literal and rows whose field is NULL, and emits the field's SQL once.
- `>`, `>=`, `<`, `<=`, `BETWEEN`, and `CONTAINS` never match a NULL field.
- Numeric literals are bound as exact decimal strings and cast with `::numeric`.
- Boolean equality compares with a bound `true` or `false` rather than emitting `IS TRUE`.
- Dates (`YYYY-MM-DD`) and timestamps (RFC 3339 with an offset) are bound as `time.Time`. UUIDs (canonical hyphenated hex) and enums (a declared value) are bound as strings; PostgreSQL infers their type from the column.
- `BETWEEN 'a,b'` is inclusive, with spaces around each bound trimmed. `BETWEEN (a, b)` compiles to the same `x BETWEEN $1 AND $2`, but each bound keeps the kind it was written as, is converted like the value of `=`, and is not trimmed: `age BETWEEN (30, 40)` binds the integers 30 and 40, `due BETWEEN ('2026-01-01', '2026-02-01')` binds two dates, and `age BETWEEN ('30', '40')` fails at the first bound because it is a string.
- NULL ordering follows PostgreSQL defaults: last when ascending, first when descending.

Expression values (`TODAY`, `NOW`, `{TODAY + 5::DAYS}`, `{10 / 4}`, ...):

- A driver never evaluates an expression. It binds the result that the easylist-core planner stored on the plan, read with `Plan.Value`. A plan without that result fails at the literal's position.
- The SQL never uses `now()`, `CURRENT_DATE`, `AT TIME ZONE`, or any other database clock or time zone function, so results do not depend on the database session. Compiling the same parsed filter with another `Request.Now` binds other values in the same SQL.
- A number result is bound like a number literal of the same text: `price > {10 / 4}` binds `'2.5'::numeric`, and `qty > {10 / 4}` on an integer field fails at the literal's position because `2.5` is not an integer.
- A date result on a date field is bound like a `YYYY-MM-DD` literal: `due > {TODAY + 5::DAYS}` binds the date five days after today in the request's zone.
- A timestamp result on a timestamp field is bound as a `time.Time` instant truncated to microseconds, PostgreSQL's precision.
- A date result `D` on a timestamp field covers the whole day `D` in the request's zone. With `S` the instant the day starts and `E` the instant the next day starts (one calendar day later, so 23 or 25 hours across a DST change), both bound as `time.Time` instants:

  | Filter | SQL |
  | --- | --- |
  | `x > D` | `x >= E` |
  | `x >= D` | `x >= S` |
  | `x < D` | `x < S` |
  | `x <= D` | `x < E` |
  | `x = D` | `(x >= S AND x < E)` |
  | `x != D` | `COALESCE(NOT (x >= S AND x < E), TRUE)` |

  So `created_at <= END_OF_WEEK` includes all of Sunday, and `created_at != TODAY` also matches NULL, like other `!=` comparisons. `=` and `!=` emit the field's SQL twice. Fixed string values such as `created_at > '2026-10-27T00:00:00Z'` keep their plain comparison.

Array fields:

- An array field is selected as `to_json(x)::text AS "name"`, so the runtime receives JSON array text and a NULL array stays NULL. The `::text` cast makes every `database/sql` driver return a string.
- On an array field a quoted literal is a comma-separated list: `ids CONTAINS '7, 8'` binds the integers `7` and `8`. Items are trimmed, an empty item is an error, and each item is checked like a scalar literal of the element type, so a list on an enum array accepts only declared values. An unquoted number or boolean is one value, as in `ids CONTAINS 7`.
- A parenthesized list gives one value per item, never split or trimmed: `tags CONTAINS ('a,b', ' c')` binds `a,b` and ` c`. Each item keeps the kind it was written as and is checked like a scalar literal of the element type, with errors at the item's position, so `ids CONTAINS (7, 'x')` fails at `'x'`. Otherwise it compiles like the quoted form: `statuses CONTAINS ('ok', 'passed')` produces the same SQL and arguments as `statuses CONTAINS 'ok,passed'`.
- `CONTAINS` binds each value once and compiles to `($1 = ANY(x) AND $2 = ANY(x))`: the array must hold every value, compared exactly and case-sensitively. A NULL array never matches.
- `EXCLUDES` compiles to `COALESCE(NOT ($1 = ANY(x) OR $2 = ANY(x)), TRUE)`: the array must hold none of the values. Empty arrays and NULL arrays match.
- `= NULL` and `!= NULL` compile to `IS NULL` and `IS NOT NULL` as for scalars. An empty array is not NULL. Every other operator, `IN` and `NOT IN` included, is rejected on arrays.
- `= ANY` lets PostgreSQL infer each argument's type from the column's element type, so the same filter works on `int4[]` and `int8[]`, `varchar[]` and `text[]`, and PostgreSQL enum arrays, with no array cast. Numeric values keep their `::numeric` cast.
- A list of N values repeats the field's SQL N times. These predicates cannot use GIN array indexes, so they scan the rows the other conditions leave.

Operators and literal checks per type are listed in [easylist-core's README](../easylist-core/README.md#types). Errors name the field and include the 1-based byte position in the filter text.

## Run the conformance suite

Add one test to the driver module:

```go
func TestConformance(t *testing.T) {
	conformance.Run(t, mydriver.New())
}
```

Only driver tests import `conformance`, so the driver's own packages never link the PostgreSQL wire driver the suite uses (`github.com/jackc/pgx/v5/stdlib`).

Every case of the suite is a subtest named after it:

- `<case>/offline` always runs. It compiles the case as a page, count, and export plan and checks that the arguments and their Go types equal `sqlpg.Lower(plan).Args()`, that the placeholders are exactly `$1` to `$n` for `n` arguments with `::numeric` only after numeric ones, that a second compile is byte-identical, that the SQL ends without `;` and reads no database clock, and, for invalid cases, that the error text equals `Lower`'s.
- `<case>/live` runs the page (or export) and count queries on PostgreSQL and compares the ids in order, the selected values, including array JSON text and SQL NULL, and the count. It is skipped when `EASYLIST_PG_DSN` is unset. When the variable is set and the database cannot be reached, the suite fails. Cases that expect an error have no live subtest.

Each run creates its own schema, `easylist_conf_<random>`, with the fixture tables, enum types, and rows, uses only that schema and the UTC session time zone, and drops it when the test ends. Several driver modules can therefore run the suite against one database at the same time.

From a driver module, or from this module, whose own tests run the suite with a reference driver:

```sh
go test -race ./...
EASYLIST_PG_DSN='postgres://postgres@127.0.0.1:54329/postgres?sslmode=disable' go test -race ./...
```

The first command runs the offline checks and reports the live subtests as skipped. The second also runs the live ones.

## Start a local PostgreSQL

The suite needs PostgreSQL 16 or later and a role that can create schemas and types. `EASYLIST_PG_DSN` takes any connection string `pgx` accepts, usually a URL of the form `postgres://<user>[:<password>]@<host>:<port>/<database>?sslmode=disable`.

With the PostgreSQL 16 server binaries installed, as on Debian and Ubuntu under `/usr/lib/postgresql/16/bin`, create a throwaway cluster with trust authentication on port 54329. `initdb` refuses to run as root, so run both commands as the `postgres` OS user, with a data directory that user owns:

```sh
sudo -u postgres /usr/lib/postgresql/16/bin/initdb -D /tmp/easylist-pg -U postgres --auth=trust
sudo -u postgres /usr/lib/postgresql/16/bin/pg_ctl -D /tmp/easylist-pg -o "-p 54329 -k /tmp" -l /tmp/easylist-pg/server.log -w start
export EASYLIST_PG_DSN='postgres://postgres@127.0.0.1:54329/postgres?sslmode=disable'
```

Stop it and remove it when done:

```sh
sudo -u postgres /usr/lib/postgresql/16/bin/pg_ctl -D /tmp/easylist-pg -w stop
rm -rf /tmp/easylist-pg
```

With Docker instead, the same `EASYLIST_PG_DSN` works:

```sh
docker run --rm -d --name easylist-pg -e POSTGRES_HOST_AUTH_METHOD=trust -p 54329:5432 postgres:16
docker stop easylist-pg
```

## Run unit tests

From the `easylist-sqlpg` directory, run:

```sh
go vet ./...
go test -race ./...
```
