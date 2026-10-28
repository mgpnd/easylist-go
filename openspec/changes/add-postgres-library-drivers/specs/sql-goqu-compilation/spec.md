# Spec Delta

## ADDED Requirements

### Requirement: PostgreSQL contract with goqu
The goqu driver SHALL satisfy `contract-sql-postgres` and pass the shared conformance suite.

#### Scenario: Conformance suite
- **WHEN** the goqu driver's tests run the conformance suite
- **THEN** every offline case passes, and every database case passes when `EASYLIST_PG_DSN` is set

### Requirement: No other query libraries
The goqu driver package, without its tests, SHALL NOT import, directly or transitively, the query library of another easylist driver or a PostgreSQL wire driver. Its tests MAY import more.

#### Scenario: Compiled-in packages
- **WHEN** the transitive imports of the goqu driver package, without tests, are listed
- **THEN** they include goqu packages and no bob, go-sqlbuilder, jet, ent, gorm, or PostgreSQL wire driver package

### Requirement: Unchanged goqu output
Moving the PostgreSQL rules into the shared module SHALL NOT change the goqu driver's SQL text, arguments, or error text for any plan.

#### Scenario: README example
- **WHEN** the goqu README example plan is compiled
- **THEN** the SQL and arguments are exactly those the README shows

#### Scenario: Existing golden tests
- **WHEN** the goqu driver's golden SQL and error tests from before this change run against the refactored driver
- **THEN** they pass without changes to their expected values

## REMOVED Requirements

### Requirement: PostgreSQL prepared output
**Reason**: It applies to every PostgreSQL driver, so it moves to `contract-sql-postgres` under the same name.
**Migration**: None. The goqu driver keeps this behaviour through the requirement "PostgreSQL contract with goqu".

### Requirement: Page query shape
**Reason**: It applies to every PostgreSQL driver, so it moves to `contract-sql-postgres` under the same name.
**Migration**: None. The goqu driver keeps this behaviour through the requirement "PostgreSQL contract with goqu".

### Requirement: Count query shape
**Reason**: It applies to every PostgreSQL driver, so it moves to `contract-sql-postgres` under the same name.
**Migration**: None. The goqu driver keeps this behaviour through the requirement "PostgreSQL contract with goqu".

### Requirement: PostgreSQL type handling
**Reason**: It applies to every PostgreSQL driver, so it moves to `contract-sql-postgres` under the same name.
**Migration**: None. The goqu driver keeps this behaviour through the requirement "PostgreSQL contract with goqu".

### Requirement: PostgreSQL NULL handling
**Reason**: It applies to every PostgreSQL driver, so it moves to `contract-sql-postgres` under the same name.
**Migration**: None. The goqu driver keeps this behaviour through the requirement "PostgreSQL contract with goqu".

### Requirement: Concurrent use
**Reason**: It applies to every PostgreSQL driver, so it moves to `contract-sql-postgres` under the same name.
**Migration**: None. The goqu driver keeps this behaviour through the requirement "PostgreSQL contract with goqu".

### Requirement: PostgreSQL array selection
**Reason**: It applies to every PostgreSQL driver, so it moves to `contract-sql-postgres` under the same name.
**Migration**: None. The goqu driver keeps this behaviour through the requirement "PostgreSQL contract with goqu".

### Requirement: PostgreSQL array membership
**Reason**: It applies to every PostgreSQL driver, so it moves to `contract-sql-postgres` under the same name.
**Migration**: None. The goqu driver keeps this behaviour through the requirement "PostgreSQL contract with goqu".

### Requirement: No database clock for relative values
**Reason**: It applies to every PostgreSQL driver, so it moves to `contract-sql-postgres` under the same name.
**Migration**: None. The goqu driver keeps this behaviour through the requirement "PostgreSQL contract with goqu".

### Requirement: Relative value binding
**Reason**: It applies to every PostgreSQL driver, so it moves to `contract-sql-postgres` under the same name.
**Migration**: None. The goqu driver keeps this behaviour through the requirement "PostgreSQL contract with goqu".

### Requirement: Whole-day equality SQL
**Reason**: It applies to every PostgreSQL driver, so it moves to `contract-sql-postgres` under the same name.
**Migration**: None. The goqu driver keeps this behaviour through the requirement "PostgreSQL contract with goqu".

### Requirement: PostgreSQL IN lists
**Reason**: It applies to every PostgreSQL driver, so it moves to `contract-sql-postgres` under the same name.
**Migration**: None. The goqu driver keeps this behaviour through the requirement "PostgreSQL contract with goqu".

### Requirement: PostgreSQL IN NULL handling
**Reason**: It applies to every PostgreSQL driver, so it moves to `contract-sql-postgres` under the same name.
**Migration**: None. The goqu driver keeps this behaviour through the requirement "PostgreSQL contract with goqu".
