package behavior

import _ "embed"

// Schema creates the fixture's types and tables in the current schema, the
// first entry of the connection's search_path.
//
//go:embed schema.sql
var Schema string

// Seed fills the tables Schema creates. Its header comment lists the people
// and the relative time windows each one falls in.
//
//go:embed seed.sql
var Seed string
