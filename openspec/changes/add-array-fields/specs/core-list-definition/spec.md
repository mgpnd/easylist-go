# Spec Delta

## ADDED Requirements

### Requirement: Array fields
Any SQL or calculated field SHALL be declarable as an array of its type, for every supported type. An array enum field SHALL declare its values like a scalar enum field. The field's nullability SHALL apply to the array as a whole; array elements are never nullable.

#### Scenario: Array of each type
- **WHEN** a list declares array fields of types text, integer, float, numeric, boolean, date, timestamp, uuid, and enum
- **THEN** the definition is accepted

#### Scenario: Array enum without values
- **WHEN** an array enum field `statuses` declares no values
- **THEN** building the list fails with an error naming `statuses`

#### Scenario: Calculated array field
- **WHEN** a calculated text field `labels` is declared with a function returning a slice of strings
- **THEN** the definition is accepted and `labels` is an array field

#### Scenario: Calculated array field with a mismatched result
- **WHEN** a calculated array field of type integer is declared with a function returning a slice of strings
- **THEN** building the list fails with an error naming the field and both Go types

### Requirement: Array primary key rejected
A list SHALL NOT name an array field as its primary key.

#### Scenario: Array primary key
- **WHEN** a list names array field `ids` as its primary key
- **THEN** building the list fails with an error naming `ids`
