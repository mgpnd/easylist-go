# Design

## Context

See proposal.md for motivation. Every SQL field value reaches a `Row` through one loop in `Runtime.Stream`. It scans each column into a holder picked by `newHolder`, which is `sql.NullTime` for date and timestamp scalars and `sql.NullString` for any array. Then `holderValue` unwraps the value, and array text goes through `decodeArray`. Nothing in that path looks at the `time.Time` location:

- **Scalar timestamps** keep whatever location the `database/sql` driver gives. pgx `stdlib` gives `time.Local` for `timestamptz` and UTC for `timestamp`. Other drivers differ: lib/pq, for one, uses a fixed zone with the session offset.
- **Array timestamps** are parsed with `time.Parse` from the JSON text that `to_json(...)::text` renders. PostgreSQL writes `timestamptz` elements with the session `TimeZone` offset. `time.Parse` then returns UTC only for `Z`. It returns `time.Local` when the offset matches the local zone at that instant, and an unnamed fixed zone otherwise. Elements without an offset (`timestamp` columns) are read as UTC.
- **Dates** come back as midnight UTC from pgx scalars, from array JSON (`time.Parse(time.DateOnly, …)`), and from `UnmarshalDate`.

Measured on the running PostgreSQL 16 from easylist-behavior's compose file, pgx v5.11 `stdlib`, value `timestamptz '2026-10-25 10:00:00+02'`, written through gqlgen's `MarshalTime`:

| Go `TZ` | session `TimeZone` | scalar field | array element |
|---|---|---|---|
| Europe/Moscow | UTC | `2026-10-25T11:00:00+03:00` | `2026-10-25T08:00:00Z` |
| Europe/Moscow | Asia/Tokyo | `2026-10-25T11:00:00+03:00` | `2026-10-25T17:00:00+09:00` |
| UTC | UTC | `2026-10-25T08:00:00Z` | `2026-10-25T08:00:00Z` |
| UTC | Asia/Tokyo | `2026-10-25T08:00:00Z` | `2026-10-25T17:00:00+09:00` |
| Asia/Tokyo | Asia/Tokyo | `2026-10-25T17:00:00+09:00` | `2026-10-25T17:00:00+09:00` |

In every case, date and `timestamp` (without zone) values came back in UTC.

The export encoders already call `UTC()` before writing timestamps, so the GraphQL `Time` scalar is the only built-in consumer that shows the zone.

## Goals / Non-Goals

**Goals:**
- One rule for SQL timestamp values, scalar and array alike, that does not depend on the Go process zone, the database session zone, or the `database/sql` driver.
- Keep the fix in the one place every driver's rows pass through, so no driver or consumer needs to know about it.

**Non-Goals:**
- Changing how dates are represented.
- Converting calculated field results.
- Changing the zone the database evaluates SQL in. Field expressions such as `created_at::date` still follow the session `TimeZone`.
- Adding a configurable output zone. A consumer that wants local times converts with `In(loc)`, as export does with the request's offset for relative values.

## Decisions

**Normalize in core, not in drivers or in gqlgen.** `Runtime.Stream` converts a scanned scalar timestamp with `UTC()` before storing it. `decodeArray` converts each parsed timestamp element the same way. Both sit on the path every row takes, and the change touches two lines plus tests.
- *Alternative: have the gqlgen `Time` marshaler call `UTC()`.* Rejected. The scalar is gqlgen's built-in one, which the plugin does not own, and fixing it there would leave `Row.Time` inconsistent for every other consumer.
- *Alternative: tell applications to set `TimestamptzCodec{ScanLocation: time.UTC}` and a UTC session zone.* Rejected as the fix, though still valid for applications. It is pgx-specific, two settings far apart, and easy to miss. The inconsistency would remain the library's default behaviour.
- *Alternative: only document the current behaviour.* Rejected. The array zone comes from the database session and the scalar zone from the Go process, so no single sentence describes what a caller gets. The library's typed-row contract exists to hide exactly that kind of difference.

**UTC, not `time.Local`.** UTC is what dates, `timestamp` columns, `Z` array elements, `UnmarshalDate`, export, and the conformance suite already use. Converting to `time.Local` would make GraphQL output depend on the server's `TZ` everywhere, which is the problem this change removes.

**Leave dates alone.** `UTC()` keeps the instant, so a date scanned as midnight in UTC+03:00 would become 21:00 on the previous day, and `MarshalDate` would print the wrong day. Every path seen today already yields midnight UTC. Re-anchoring the wall date to UTC midnight would be the right fix if a driver ever differs, but nothing needs it yet. The spec scenario "Date keeps its calendar day" pins the requirement that matters.

**Leave calculated values alone.** A calculated field's function belongs to the application, which may return a zone on purpose. Core only promises a zone for values it reads from the database. Export still writes calculated timestamps in UTC through its own `UTC()` calls, which therefore stay.

**easylist-behavior's workaround.** The harness registers `TimestamptzCodec{ScanLocation: time.UTC}` on every connection and sets `timezone=UTC` as a runtime parameter. After this change the codec is redundant and is removed. The session parameter stays: it still decides how PostgreSQL evaluates zone-dependent SQL, and the conformance suite sets it too. Golden files must not change. If one does, a timestamp is reaching a snapshot by a path that skips the runtime, and that has to be understood before the codec goes.

## Risks / Trade-offs

- [An application formatted `Row.Time` directly and relied on pgx's `time.Local`] → Called out as **BREAKING** in the proposal and in the core README. The fix is one `In(time.Local)` call. Instants are unchanged, so filtering, sorting, and comparisons with `Equal` behave the same.
- [Tests that compare row times with `==` or `reflect.DeepEqual` against local-zone values start failing] → Same mitigation. Inside this repository, `TestDecodeArrayElements` expects a `+02:00` fixed zone today and changes with this work.
- [A third-party consumer reads values through a path other than `Runtime.Stream`, for example by building rows with `NewRow`] → Those rows hold what the caller put in them. The UTC guarantee covers values the runtime reads from the database, as the spec says.

## Migration Plan

No data or schema migration. Applications that want local display times convert at the edge. Rolling back is reverting the two `UTC()` calls.
