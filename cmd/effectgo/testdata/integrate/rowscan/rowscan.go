// Package rowscan calls Scan on the two database/sql row types of the stdlib stubs.
package rowscan

import "database/sql"

// RowsScan reads the open cursor. It never returns sql.ErrNoRows.
func RowsScan(r *sql.Rows) error { return r.Scan() }

// RowScan reads the single row of a query. It returns sql.ErrNoRows when there is no row.
func RowScan(r *sql.Row) error { return r.Scan() }
