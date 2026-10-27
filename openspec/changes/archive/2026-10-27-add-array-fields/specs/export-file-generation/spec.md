# Spec Delta

## ADDED Requirements

### Requirement: Array cells
CSV and XLSX output SHALL write an array value as one text cell. Its elements SHALL be joined by commas with no spaces, each formatted as the scalar CSV rules require. Text, enum, uuid, date, and timestamp elements SHALL be wrapped in double quotes, with embedded double quotes doubled. An empty array SHALL be an empty cell, as is a NULL array.

#### Scenario: Text array
- **WHEN** text array `tags` holds `go` and `rust`
- **THEN** the cell text is `"go","rust"`

#### Scenario: Integer array
- **WHEN** integer array `ids` holds `7` and `8`
- **THEN** the cell text is `7,8`

#### Scenario: Embedded quote and comma
- **WHEN** text array `tags` holds `say "hi"` and `a,b`
- **THEN** the cell text is `"say ""hi""","a,b"`

#### Scenario: Date and timestamp arrays
- **WHEN** date array `days` holds `2026-10-25` and timestamp array `seen_at` holds `2026-10-25T10:00:00+02:00`
- **THEN** the cells are `"2026-10-25"` and `"2026-10-25T08:00:00Z"`

#### Scenario: Numeric and boolean arrays
- **WHEN** numeric array `prices` holds `10.50` and boolean array `flags` holds `true` and `false`
- **THEN** the cells are `10.50` and `true,false`

#### Scenario: Array in CSV quoting
- **WHEN** text array `tags` holds `go` and `rust` in a CSV export
- **THEN** the raw CSV field is `"""go"",""rust"""`, which a CSV reader decodes to `"go","rust"`

#### Scenario: Array in XLSX
- **WHEN** integer array `ids` holds `7` and `8` in an XLSX export
- **THEN** the cell is a text cell holding `7,8`, not a number

#### Scenario: Empty array
- **WHEN** array `tags` holds no elements
- **THEN** its cell is empty
