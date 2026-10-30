# Proposal

## Why

The same timestamp field comes back in different time zones depending on whether it is an array and on settings outside easylist. A scalar `Timestamp` value is whatever `time.Time` the `database/sql` driver scans, and pgx `stdlib` scans `timestamptz` into the Go process's `time.Local`. An array element is parsed from the JSON text PostgreSQL renders, which carries the database session's `TimeZone` offset. The instant is always right, but the zone leaks into everything that formats the value. The GraphQL `Time` scalar writes `2026-10-25T11:00:00+03:00` for a scalar field on a Moscow machine and `2026-10-25T08:00:00Z` for the same instant in an array field. With a Tokyo session zone, the array writes `+09:00`. Responses differ across machines and deployments for the same data. The easylist-behavior snapshots had to pin both zones to get stable output.

## What Changes

- Every timestamp value the runtime reads from an SQL field is returned in UTC (`time.UTC` location), for scalar fields and for each element of a timestamp array field. The instant is unchanged; only the location changes.
- Date values are not touched. They already reach the row as midnight UTC from pgx and from array JSON, and calling `UTC()` on a date scanned at local midnight would move it to the previous day.
- Calculated timestamp fields keep the `time.Time` their function returns. The application owns those values.
- GraphQL responses therefore write SQL timestamp fields with a `Z` offset whatever the server's `TZ` or the database session zone. No change to easylist-gqlgen code: it formats what the row holds.
- CSV and XLSX export already convert timestamps to UTC and are unchanged.
- **BREAKING** (observable only through the zone): an application that formats a scalar timestamp from `Row.Time` without converting it, and relied on getting the driver's zone (for example `time.Local` with pgx), now gets UTC and must call `In(loc)` itself. `Equal`, `Before`, `Unix`, and every other instant comparison are unaffected. Code that compares with `==` or `reflect.DeepEqual` against a non-UTC `time.Time` will see a difference.
- When easylist-behavior is on main, its harness drops the pgx `TimestamptzCodec{ScanLocation: time.UTC}` registration, because core now does that work. It keeps the session `timezone=UTC` runtime parameter, which still governs SQL the database evaluates itself.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `core-list-execution`: "Typed row values" states that SQL timestamp values are in UTC whatever zone the driver scans them in, and that dates and calculated values are not converted. "Array row values" makes the timestamp array scenario require UTC elements whatever the database session zone.
- `gqlgen-schema-generation`: "GraphQL type mapping" gains a scenario that a timestamp SQL field is written with a `Z` offset whatever the server's local zone.

## Impact

- **easylist-core**: `Runtime.Stream` (`runtime.go`) converts scalar timestamp values to UTC after scanning. `decodeArray` (`array.go`) converts each timestamp element to UTC. Tests in `runtime_test.go` and `array_test.go`, and the README types and arrays sections, change with them.
- **easylist-gqlgen**: no code change. A test pins the `Z` output for a timestamp field whose row value came from a non-UTC scan.
- **easylist-export**: no code change. Its `UTC()` calls stay, because they also cover calculated timestamp values.
- **easylist-behavior** (on branch `claude/project-behavior-visibility-4d6355` until it merges): `harness_test.go` drops the custom timestamptz codec and the `pgtype` import. Its golden files are expected to stay byte-identical.
- **easylist-filter**, **easylist-sqlpg**, **easylist-sql-bob**, **-ent**, **-goqu**, **-gorm**, **-jet**, **-sqlbuilder**: untouched. Drivers still select arrays as JSON text and leave scalar scanning to the database handle.
