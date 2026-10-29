package sqljet_test

import (
	"testing"

	sqljet "github.com/mgpnd/easylist-go/easylist-sql-jet"
	"github.com/mgpnd/easylist-go/easylist-sqlpg/conformance"
)

// The driver passes the shared PostgreSQL suite. Its live cases run when
// EASYLIST_PG_DSN is set and are skipped otherwise.
func TestConformance(t *testing.T) {
	conformance.Run(t, sqljet.New())
}
