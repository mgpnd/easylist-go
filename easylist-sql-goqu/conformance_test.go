package sqlgoqu_test

import (
	"testing"

	sqlgoqu "github.com/mgpnd/easylist-go/easylist-sql-goqu"
	"github.com/mgpnd/easylist-go/easylist-sqlpg/conformance"
)

// The driver passes the shared PostgreSQL suite. Its live cases run when
// EASYLIST_PG_DSN is set and are skipped otherwise.
func TestConformance(t *testing.T) {
	conformance.Run(t, sqlgoqu.New())
}
