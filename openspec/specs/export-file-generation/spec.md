# export-file-generation Specification

## Purpose

Turns a validated export request into a CSV or XLSX file written to an application-supplied writer, so an application's job worker only has to manage job lifecycle and storage.

## Requirements

### Requirement: Worker entry point
The library SHALL provide a function that takes a context, the list registry, the runtime (driver and database), an export request, and an output writer, and writes the complete file to the writer or returns an error. It SHALL NOT queue work, track job status, or store files.

#### Scenario: Worker writes a file
- **WHEN** an application worker calls the function with a valid CSV export request and a file writer
- **THEN** the file contains the exported rows and the function returns no error

### Requirement: Revalidation in the worker
The worker SHALL validate the export request against the current list registry before querying, because definitions may change between enqueueing and running. A request that no longer validates SHALL fail without writing any rows.

#### Scenario: Field removed after enqueueing
- **WHEN** a request names `country` and the deployed `users` list no longer has `country`
- **THEN** the worker returns an error naming `country`

### Requirement: Single streaming query
The worker SHALL run exactly one query for the export plan, with no limit or offset, and write each row as it is read, so memory use does not grow with the number of exported rows.

#### Scenario: Large export
- **WHEN** an export matches one million rows
- **THEN** one query runs and rows are written while it is being read, without first collecting all rows in memory

### Requirement: Columns and header
The file SHALL have one column per requested field, in request order, and a header row holding each field's title. Calculated fields SHALL be evaluated per row with the worker's context.

#### Scenario: Header uses titles
- **WHEN** fields `name` (title `Full name`) and `display` (no title) are exported
- **THEN** the header row is `Full name`, `display`

### Requirement: CSV encoding
CSV output SHALL be UTF-8 with comma separators and RFC 4180 quoting. NULL values SHALL be empty cells. Dates SHALL be `YYYY-MM-DD`, timestamps RFC 3339 in UTC, booleans `true`/`false`, numerics their exact decimal text, and enums their declared value.

#### Scenario: Value needing quotes
- **WHEN** a text value is `Smith, "Jr"`
- **THEN** the CSV cell is `"Smith, ""Jr"""`

#### Scenario: NULL value
- **WHEN** nullable `country` is NULL
- **THEN** its CSV cell is empty

### Requirement: XLSX encoding
XLSX output SHALL contain one worksheet named after the list. Strings SHALL be written as text cells and never as formulas. Integers within ±2^53 and floats SHALL be number cells; numerics and larger integers SHALL be text cells to keep exact digits. Dates and timestamps (UTC) SHALL be date cells, booleans boolean cells, and NULL values empty cells.

#### Scenario: Formula-like text
- **WHEN** a text value is `=SUM(A1:A9)`
- **THEN** the XLSX cell holds that text, not a formula

#### Scenario: Exact numeric
- **WHEN** a numeric value is `12345678901234567890.10`
- **THEN** the XLSX cell holds the text `12345678901234567890.10`

### Requirement: Failure handling
The worker SHALL stop at the first query, calculated-field, encoding, write, or context-cancellation error and return it. Output already written before the error SHALL be treated by the caller as incomplete.

#### Scenario: Calculated field fails
- **WHEN** a calculated field returns an error on row 500
- **THEN** the worker stops and returns that error wrapped with the field name
