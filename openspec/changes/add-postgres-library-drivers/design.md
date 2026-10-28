# Design

## Context

See proposal.md for why. The relevant parts of the code today:

- `easylist-sql-goqu` has three parts. `predicate.go` (~430 lines) does
  everything that depends on the plan and not on goqu: field lookup, operator
  checks, literal and list conversion, evaluated values, whole-day bounds,
  and the NULL-safe forms of `!=`, `EXCLUDES`, `IN`, and `NOT IN`. `lower.go`
  walks the filter tree without recursion, left child first. `driver.go`
  assembles the statement.
- Every predicate is already a raw `goqu.L` template with `?` slots, whose
  slot values are either the field's author SQL (a nested `goqu.L` without
  arguments, so goqu writes it verbatim) or a bound value. goqu only builds
  `SELECT`/`FROM`/`WHERE`/`ORDER BY` and numbers the placeholders. The goqu
  driver appends `LIMIT $n OFFSET $m` itself because goqu drops `OFFSET 0`.
- Every existing test compares SQL strings or arguments. None runs a query.
- The container has PostgreSQL 16 (`/usr/lib/postgresql/16/bin`, `psql` on
  `PATH`) and Docker. The repository has no CI.
- The library versions read for this design are bob v0.50.0, go-sqlbuilder
  v1.44.1, jet v2.16.1, ent v0.14.6, and gorm v1.31.2.

## Goals / Non-Goals

**Goals:**
- All plan-dependent decisions are made in one place. A driver contains no
  type, operator, or NULL logic.
- The goqu driver's output stays byte-identical, and its existing golden
  tests act as the regression check for the extraction.
- Each new driver uses its library for statement assembly and logical
  connectors, and its library's own predicate constructs wherever they meet
  the contract.
- The conformance suite can tell a driver whether its arguments are right
  without a database, so most mistakes show up in a plain `go test`.

**Non-Goals:**
- Other dialects. `easylist-sqlpg` is PostgreSQL-only. Another dialect would
  get its own shared module.
- Executing queries through each library (`bob.Exec`, `gorm.DB.Scan`, and
  so on). Drivers return SQL and arguments. `easylist.Runtime` executes them
  through its `Queryer`, as it does today.
- Using generated code from jet, ent, or bob.
- CI. The suite runs wherever `EASYLIST_PG_DSN` is set. Wiring it into CI
  can come later.

## Decisions

### `easylist-sqlpg` lowers a plan into a `Query`

```
easylist.Plan --sqlpg.Lower--> *sqlpg.Query --driver maps--> (sql, args)
                    |
                    +-- every validation error, with the text from today
```

```go
type Query struct {
    Count  bool
    From   Expr       // "<table> AS <alias> <join SQL>...", verbatim
    Select []Column   // page/export only, in plan order
    Where  Cond       // nil when there is no scope and no filter
    Order  []Ordering // page/export only
    Page   *Page      // page only: Limit, Offset int64
}
type Column   struct { Expr Expr; Alias string } // arrays already wrapped in to_json(...)::text
type Ordering struct { Expr Expr; Desc bool }
type Expr     struct { SQL string }               // author or generated SQL, never re-parsed
type Arg      struct { Value any; Cast string }   // Cast is "" or "numeric"

func Lower(p *easylist.Plan) (*Query, error)
func (q *Query) Args() []any // expected arguments, contract order, limit/offset last
```

`Where` holds the scope as a `Raw` node ANDed in front of the filter, as
`driver.go` does today. Field expressions come in two forms, plain for
`SELECT`/`ORDER BY` and parenthesized for `WHERE`, matching
`fieldExpression`.

Alternatives considered:
- *Export the current helpers (`predicateArgument`, `listArguments`, ...)
  and have each driver write its own `buildPredicate`*: the operator and
  NULL logic would still be copied into every driver.
- *Put the code in `easylist-core`*: core is dialect-neutral, and the
  PostgreSQL forms (`IS DISTINCT FROM`, `ILIKE ... ESCAPE`, `= ANY`,
  `::numeric`) don't belong there.

### Typed conditions, each with a canonical fragment

```go
type Cond interface{ Fragment() Fragment }

And{Left, Right}  Or{Left, Right}  Raw{SQL}              // scope
Compare{Col, Op, Arg}       // Op: = IS-DISTINCT-FROM > >= < <=
IsNull{Col, Not}
In{Col, Args, Not, Null}    // Null: NullKeep | NullMatch | NullNoMatch
Between{Col, Low, High}
ILike{Col, Pattern, Not}    // Pattern is already escaped and wrapped in %...%
ArrayHas{Col, Values, None} // CONTAINS (all) / EXCLUDES (none)
DayRange{Col, Start, End, Not}

type Fragment struct {
    SQL   string // "?" marks each slot, in order
    Slots []any  // Expr (written verbatim) or Arg (bound)
}
```

Every `Fragment()` reproduces the template that `predicate.go` uses today.
For example, `In{Not: true, Null: NullMatch}` gives
`COALESCE(? NOT IN (?, ?), TRUE)` with slots `[Col, a, b]`. So a driver that
only renders fragments emits exactly the goqu SQL, apart from whitespace.

A driver walks the `Cond` tree. It maps `And`/`Or` to its library's
connectors, maps any other node to a native construct when one exists that
gives the same semantics with no cast and no extra argument, and otherwise
renders `Fragment()` with its library's raw-with-arguments primitive. The
node types are exported so that drivers can choose. The fragment is the
reference: a driver's golden tests compare each native mapping with it.

Alternatives considered:
- *Fragments only, with no typed nodes*: simpler, but the libraries would do
  nothing beyond assembly, which is what the explore session rejected.
- *Native mappings required*: several libraries have no construct for
  `COALESCE(...)`, `ILIKE ... ESCAPE`, or `= ANY(...)`, and some native forms
  add casts (jet) or quote columns (ent, sqlbuilder `Cond` in some paths).

### Author SQL is always an opaque leaf

Every library has some syntax it rewrites: `?` for bob `psql.Raw`, gorm
`clause.Expr`, and goqu literals with arguments; `$` for go-sqlbuilder;
named-argument tokens for jet `Raw`. So no driver may concatenate author SQL
into a template that the library then scans. An `Expr` is always written
through a construct that copies it without scanning:

| Driver | Opaque leaf | Bound slot | Placeholder numbering |
|---|---|---|---|
| goqu | `goqu.L(sql)` with no args | slot value in `goqu.L(tpl, ...)` | `Prepared(true)` |
| bob | `expr.Raw(sql)` | `psql.Arg(v)` inside `psql.Raw(tpl, ...)` | bob psql, `$n` |
| go-sqlbuilder | `sqlbuilder.Escape(sql)` text | `sb.Var(v)` | `BuildWithFlavor(PostgreSQL)` |
| jet | `Raw(sql)` with no named args | `Raw("#v", RawArgs{"#v": v})`, one arg per `Raw` | jet postgres, `$n` |
| ent | `b.WriteString(sql)` inside `sql.P(func(b){...})` | `b.Arg(v)` | ent Postgres builder, `$n` |
| gorm | `clause.Expr{SQL: sql}` with no vars | `clause.Expr{SQL: tpl, Vars: ...}` | private dialector `BindVarTo` writes `$n` |

The fragment pieces are combined with each library's composition construct.
For jet that is `CustomExpression(Token(...), leaf, arg, ...)`. Using one
named argument per `Raw` avoids jet's substring matching between names such
as `#1` and `#10`, and it never scans author SQL.

The conformance suite pins this down with author SQL containing `?`, `$`,
`#`, and `:` (`contract-sql-postgres`, "Author SQL with placeholder
characters").

### Per-driver notes

- **goqu**: `driver.go` keeps its current assembly and renders every node
  through `Fragment()` with `goqu.L`. It uses no native goqu operators, so
  the output stays byte-identical, as `sql-goqu-compilation` requires.
- **bob** (`psql.Select` with `sm.From`, `sm.Columns`, `sm.Where`,
  `sm.OrderBy`, `sm.Limit`, `sm.Offset`): `From` is `expr.Raw`. bob binds
  `LIMIT` and `OFFSET`, including 0. It has native `IsDistinctFrom`, `In`,
  `NotIn`, `Between`, `IsNull`, and `IsNotNull`. Use them on an `expr.Raw`
  column when the golden test shows no cast. Fragments cover the rest.
- **go-sqlbuilder** (`sqlbuilder.PostgreSQL.NewSelectBuilder()`): `From` and
  `Select` take escaped author SQL. `Limit`/`Offset` are bound, including 0.
  `Cond` methods write their field argument raw, so pass the escaped column
  only where the method does not escape it itself. Golden tests check both
  paths. Native `Equal`, `IsDistinctFrom`, `In`, `NotIn`, `Between`,
  `IsNull`, and `IsNotNull` are allowed under the same no-cast rule.
- **jet** (`postgres.SELECT(...).FROM(...)`): jet cannot take a verbatim
  `FROM` with joins. `ReadableTable` has unexported methods, and `NewTable`
  quotes names containing spaces. The driver builds the statement over
  `NewTable("", sentinel, "")`, where `sentinel` is a lowercase identifier
  that jet writes unquoted. It then replaces the one occurrence of the
  sentinel in the output with `Query.From`. The sentinel starts as
  `easylist_from` and gets `_x` appended until it occurs in no author SQL of
  the plan. The driver returns an error if the sentinel does not occur
  exactly once. Projections are `Raw(expr).AS(alias)`, predicates are built
  with `CustomExpression` (jet's typed literals add `::text` casts, so they
  are never used), and `LIMIT`/`OFFSET` are bound. The trailing `;\n` is
  trimmed.
- **ent** (`sql.Dialect(dialect.Postgres).Select()`): `FromExpr(sql.Raw(from))`,
  `AppendSelectExprAs(sql.Raw(expr), alias)`, `OrderExpr(sql.Raw(...))`, and
  `Where(...)` with `sql.And`/`sql.Or` over `sql.P` predicates. ent's native
  predicates quote column names, so only fragments are used. `Limit`/`Offset`
  write numbers into the SQL, so the driver appends
  `LIMIT $n OFFSET $m` with `len(args)+1` and `+2`, as goqu does.
- **gorm**: a package-level `*gorm.DB` opened once with a private dialector
  and `DryRun: true`. The dialector implements `BindVarTo` (writes `$n`) and
  `QuoteTo` (writes `"name"`), registers the default callbacks, and
  implements the rest of `gorm.Dialector` as no-ops. Each `Compile` starts a
  new `Session`, sets `TableExpr` to the verbatim `From`, adds
  `clause.Select`, `clause.Where`, and `clause.OrderBy` built from
  `clause.Expr`, and reads `Statement.SQL` and `Statement.Vars` after
  `Find`. gorm drops `Offset(0)`, so `LIMIT $n OFFSET $m` is appended by
  hand. gorm runs a `?` directly after `(` through `processValue`, which
  expands slices. No argument is a slice, and a golden test pins the
  `IN (...)` form.

### Errors come only from `Lower`

`Lower` returns every validation error with the same text as today's goqu
driver. A driver may only add errors for failures inside its library, such
as jet's sentinel check, and wraps them with the list name. That gives the
"Identical errors across drivers" requirement without per-driver work.

### The conformance suite

```
easylist-sqlpg/conformance
  Run(t *testing.T, d easylist.Driver)
    offline (always):
      for each case: Compile -> args == sqlpg.Lower(plan).Args()
                                 (same values and types)
                     placeholders $1..$len(args), each used
                     second Compile is byte-identical
                     error cases: error text == Lower's error
    live (EASYLIST_PG_DSN set, else t.Skip in that subtest):
      CREATE SCHEMA easylist_conf_<random>; SET search_path
      fixture DDL + seed rows (enum, uuid, numeric, date, timestamptz,
                               timestamp, jsonb, text[]/int8[]/enum[] arrays,
                               NULLs and empty arrays, a countries join)
      for each case: run page query  -> ids in order (and selected values)
                     run count query -> count == len(expected ids)
      DROP SCHEMA ... CASCADE (t.Cleanup)
```

- Cases are Go values in the `conformance` package. Each has a name, filter
  text, an optional scope or fields, a reference time and time zone, and
  either the expected ids or the expected error. One list definition over
  the fixture backs every case, so cases stay short.
- The connection uses `database/sql` with `github.com/jackc/pgx/v5/stdlib`.
  Only the `conformance` package imports it. Driver packages never import
  `conformance`, so a driver's compiled-in packages stay free of a wire
  driver. Only driver tests import it.
- Expected rows are written by hand from the contract scenarios. They are
  not taken from goqu's output, so the suite checks the contract and not
  the reference implementation.
- `easylist-sqlpg/README.md` documents running PostgreSQL 16 locally with
  `initdb`/`pg_ctl` or Docker, and the `EASYLIST_PG_DSN` format.

### Module and package names

| Module | Package | Library import |
|---|---|---|
| `easylist-sqlpg` | `sqlpg` | none |
| `easylist-sql-bob` | `sqlbob` | `github.com/stephenafamo/bob` |
| `easylist-sql-sqlbuilder` | `sqlsqlbuilder` | `github.com/huandu/go-sqlbuilder` |
| `easylist-sql-jet` | `sqljet` | `github.com/go-jet/jet/v2` |
| `easylist-sql-ent` | `sqlent` | `entgo.io/ent` |
| `easylist-sql-gorm` | `sqlgorm` | `gorm.io/gorm` |

These follow `sqlgoqu`: `sql` plus the library's name. For go-sqlbuilder that
gives the awkward `sqlsqlbuilder`. Naming it after the library's import name
avoids a clash with the library's own `sqlbuilder` package in user code.
Every driver exposes `New() *Driver` and `var _ easylist.Driver = (*Driver)(nil)`.

## Risks / Trade-offs

- [The goqu extraction changes behaviour subtly] → goqu's golden SQL,
  argument, and error tests stay unchanged and must pass before any new
  driver is started. The validation tests move to `easylist-sqlpg` with
  their expected values unchanged.
- [jet's sentinel replacement is a string hack] → the sentinel is chosen to
  be absent from all author SQL, its occurrence count is checked, and the
  conformance suite runs joins and author SQL with unusual characters
  through it. If jet later gains a raw-table construct, the hack goes away
  without changing the spec.
- [bob is pre-1.0 and changes its API between minor versions] → pin the
  version in `go.mod`. The driver uses a small surface (`psql.Select`, `sm`,
  `expr.Raw`, `psql.Raw`, `psql.Arg`).
- [gorm's internals (`Statement.SQL`, the DryRun callbacks) are not a
  stable API] → pin the version, and keep the gorm-specific code to one
  file with golden tests for every clause.
- [ent pulls in a large module graph] → only `entgo.io/ent/dialect` and
  `dialect/sql` are imported. The compiled-in package check in
  `sql-ent-compilation` keeps it that way.
- [Without CI, the live suite only runs where someone sets a DSN] → the
  offline half catches argument and error regressions in every `go test`.
  The task list runs the live half once in this container before
  archiving.
- [Native predicate mappings can drift from the fragment semantics] → the
  same cases run against every driver on a live database, and the
  no-cast/argument-order checks run offline.
- [`go.work` requires Go 1.25 and the container has 1.24.7] → rely on
  `GOTOOLCHAIN=auto` to download 1.25. If that fails, the first task stops
  and reports it.

## Migration Plan

There is no published API to migrate. `sqlgoqu.New()` and its output are
unchanged. The new modules are additive. Rollback means reverting the
change. The only cross-module code move is from `easylist-sql-goqu` into
`easylist-sqlpg`, and it is done in one task group so that the workspace
builds after every group.

## Open Questions

- Whether bob's and go-sqlbuilder's native `In`/`Between` keep the column
  emitted once and add no cast in every case. The golden tests decide per
  node, and the fragment is the fallback, so the answer changes neither the
  specs nor the tasks.
