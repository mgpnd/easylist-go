# Proposal

## Why

Lists often expose multi-valued columns: tags, assigned user ids, status histories. easylist has no array type today, so these columns cannot be listed, exported, or filtered. Users need to filter them by membership, for example "statuses include `passed`" or "statuses include neither `skipped` nor `failed`".

## What Changes

- Any SQL or calculated field can be declared an array of its type with a new `Array` flag. This covers text, integer, float, numeric, boolean, date, timestamp, uuid, and enum. Array values are returned as Go slices of the element's existing representation, such as `[]string` or `[]int64`.
- Array elements are never NULL; reading a NULL element fails the request. A nullable array field may itself be NULL, and an empty array is a value, not NULL.
- Drivers deliver array values to `easylist-core` as JSON array text, which core decodes without any database-specific dependency.
- Array fields accept these filter operators, and only these:
  - `x CONTAINS v` matches rows whose array has an element equal to `v`.
  - `x CONTAINS 'a,b'` matches rows whose array contains every listed value.
  - `x EXCLUDES v` and `x EXCLUDES 'a,b'` match rows whose array contains none of the listed values, and rows where the field is NULL.
  - `x = NULL` and `x != NULL`, from the `null-filters-and-excludes` change.
- On array fields, a quoted string is always a comma-separated list. Items are trimmed, an empty item is an error, and each item is converted to the element type. Non-text arrays need a quoted list for several values, as in `ids CONTAINS '7,8'`. A single unquoted value such as `ids CONTAINS 7` still works.
- Text fields keep their current `CONTAINS`/`EXCLUDES` behaviour: a case-insensitive substring search, with no list splitting.
- Array fields cannot be sorted.
- GraphQL exposes array fields as `[T!]`, or `[T!]!` when the field is not nullable.
- CSV and XLSX exports write an array as one text cell. Elements are joined with commas, and text, enum, uuid, date, and timestamp elements are wrapped in double quotes with embedded quotes doubled, for example `"go","rust"` or `7,8`. An empty array is an empty cell.

Modules touched:
- `easylist-core`: the `Array` flag, its validation, the `Plan` field metadata, JSON decoding of array values, `Row` accessors, sort rejection, and the README.
- `easylist-sql-goqu`: selecting arrays as JSON, array `CONTAINS`/`EXCLUDES`/NULL compilation, list literal conversion, and the README.
- `easylist-gqlgen`: SDL list types, generated Go row fields and enum slices, the sort enum, and the README.
- `easylist-export`: CSV and XLSX array cells, and the README.

`easylist-filter` is not touched: lists are ordinary string literals.

This change depends on `null-filters-and-excludes`, which supplies `= NULL`, `!= NULL`, and NULL-inclusive `EXCLUDES`. That change must be applied first.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `core-list-definition`: fields can be declared as arrays of any supported type, and array fields cannot be the primary key.
- `core-list-execution`: array fields return slices, NULL elements are rejected, and enum elements must be declared values.
- `core-query-planning`: sorting on an array field fails planning.
- `contract-sql-driver`: array values reach core as JSON array text, and there are new array operators, list literals, and membership semantics.
- `sql-goqu-compilation`: PostgreSQL compilation of array selection and array membership predicates.
- `gqlgen-schema-generation`: array fields map to GraphQL list types and are left out of the sort enum.
- `export-file-generation`: how an array is written to CSV and XLSX cells.

## Impact

- **Public API (`easylist-core`)**:
  - `Field.Array` and `PlanField.Array`.
  - New `Row` accessors: `Strings`, `Int64s`, `Float64s`, `Bools`, and `Times`.
  - `Calc` and `CalcNullable` accept slice result types for array fields.
- **Driver contract**: a third-party driver must select array fields as JSON array text. No third-party drivers are known.
- **Generated GraphQL schema**: new list-typed fields appear only when a list declares array fields. Existing schemas are unchanged.
- **Dependencies**: none added. Decoding uses the standard library's `encoding/json`.
