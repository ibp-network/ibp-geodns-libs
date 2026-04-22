package requestschema

import (
	"database/sql"
	"fmt"
)

const UniqueIndexName = "uniq_traffic_dedupe"

var expectedUniqueIndexColumns = []string{
	"date",
	"node_id",
	"domain_name",
	"member_name",
	"network_asn",
	"network_name",
	"country_code",
	"country_name",
	"is_ipv6",
}

func ExpectedUniqueIndexColumns() []string {
	out := make([]string, len(expectedUniqueIndexColumns))
	copy(out, expectedUniqueIndexColumns)
	return out
}

func HasExpectedUniqueIndex(columns []string) bool {
	if len(columns) != len(expectedUniqueIndexColumns) {
		return false
	}

	for i := range columns {
		if columns[i] != expectedUniqueIndexColumns[i] {
			return false
		}
	}

	return true
}

func CurrentUniqueIndexColumns(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`
SELECT COLUMN_NAME
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'requests'
  AND INDEX_NAME = ?
ORDER BY SEQ_IN_INDEX
`, UniqueIndexName)
	if err != nil {
		return nil, fmt.Errorf("query requests index metadata: %w", err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, fmt.Errorf("scan requests index metadata: %w", err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate requests index metadata: %w", err)
	}

	return columns, nil
}

func EnsureUniqueIndex(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("nil DB")
	}

	columns, err := CurrentUniqueIndexColumns(db)
	if err != nil {
		return err
	}
	if HasExpectedUniqueIndex(columns) {
		return nil
	}

	ddl := `
ALTER TABLE requests
`
	if len(columns) > 0 {
		ddl += "DROP INDEX " + UniqueIndexName + ",\n"
	}
	ddl += `
ADD UNIQUE KEY uniq_traffic_dedupe (
  date, node_id, domain_name, member_name,
  network_asn, network_name, country_code,
  country_name, is_ipv6
)`

	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("update requests unique index: %w", err)
	}

	return nil
}
