package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	sqlFile := flag.String("sql", "", "Path to MySQL dump file (EQOA_Master.sql)")
	dbFile := flag.String("db", "eqoa.db", "Path to SQLite database file")
	flag.Parse()

	if *sqlFile == "" {
		log.Fatal("Usage: sqlimport -sql <path/to/EQOA_Master.sql> -db <path/to/eqoa.db>")
	}

	// Remove existing database for fresh import
	if _, err := os.Stat(*dbFile); err == nil {
		log.Printf("Removing existing database: %s", *dbFile)
		os.Remove(*dbFile)
	}

	// Open SQLite database
	db, err := sql.Open("sqlite", *dbFile+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(1)

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		log.Fatalf("Failed to set pragma: %v", err)
	}

	// Read SQL file
	f, err := os.Open(*sqlFile)
	if err != nil {
		log.Fatalf("Failed to open SQL file: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Increase buffer size for very long lines (INSERT INTO with thousands of rows)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024) // 10MB max line

	var (
		currentStmt strings.Builder
		inCreate    bool
		tableCounts = make(map[string]int64)
		lineNum     int
	)

	// Begin transaction for speed
	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("Failed to begin transaction: %v", err)
	}

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip MySQL-specific directives
		if shouldSkipLine(line) {
			continue
		}

		// Handle CREATE TABLE
		if strings.HasPrefix(line, "CREATE TABLE") {
			inCreate = true
			currentStmt.Reset()
			// Convert CREATE TABLE line
			converted := convertCreateTableLine(line)
			currentStmt.WriteString(converted)
			currentStmt.WriteString("\n")
			continue
		}

		if inCreate {
			if strings.HasPrefix(line, ")") {
				// End of CREATE TABLE - close it
				currentStmt.WriteString(");\n")
				inCreate = false

				stmt := currentStmt.String()
				stmt = cleanCreateTable(stmt)

				if _, err := tx.Exec(stmt); err != nil {
					log.Printf("Line %d: CREATE TABLE error: %v", lineNum, err)
					log.Printf("Statement: %s", stmt[:min(500, len(stmt))])
				} else {
					tableName := extractTableName(stmt)
					log.Printf("Created table: %s", tableName)
				}
				currentStmt.Reset()
				continue
			}

			// Column or key definition inside CREATE TABLE
			converted := convertColumnLine(line)
			if converted != "" {
				currentStmt.WriteString(converted)
				currentStmt.WriteString("\n")
			}
			continue
		}

		// Handle INSERT statements (skip stored procedure INSERT bodies that lack VALUES with data)
		if strings.HasPrefix(line, "INSERT INTO") || strings.HasPrefix(line, "INSERT into") {
			// Skip stored procedure INSERT templates (they reference parameters, not data)
			if !strings.Contains(line, "VALUES (") && !strings.Contains(line, " VALUES ") {
				continue
			}
			// Real INSERTs have VALUES with numeric/string literals like (1,'WAR',...) not (paramName, ...)
			// Stored proc INSERTs reference variable names. Quick heuristic: if VALUES line
			// doesn't start with a number, quote, or NULL, skip it.
			valIdx := strings.Index(line, "VALUES (")
			if valIdx == -1 {
				valIdx = strings.Index(line, "VALUES(")
			}
			if valIdx >= 0 {
				afterValues := strings.TrimSpace(line[valIdx+7:])
				if len(afterValues) > 1 {
					afterValues = strings.TrimPrefix(afterValues, "(")
					firstChar := afterValues[0]
					// Real data starts with digit, minus sign, single quote, or NULL
					if firstChar != '-' && firstChar != '\'' && !(firstChar >= '0' && firstChar <= '9') && !strings.HasPrefix(afterValues, "NULL") {
						continue
					}
				}
			}
			stmt := convertInsert(line)
			if stmt != "" {
				result, err := tx.Exec(stmt)
				if err != nil {
					log.Printf("Line %d: INSERT error: %v", lineNum, err)
					log.Printf("Statement prefix: %.200s", stmt)
				} else {
					rows, _ := result.RowsAffected()
					tableName := extractInsertTableName(stmt)
					tableCounts[tableName] += rows
				}
			}
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Scanner error: %v", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		log.Fatalf("Failed to commit transaction: %v", err)
	}

	// Report counts
	fmt.Println("\n=== Import Summary ===")
	for table, count := range tableCounts {
		fmt.Printf("  %-25s %d rows\n", table, count)
	}

	// Also verify with direct counts
	fmt.Println("\n=== Verification (SELECT COUNT) ===")
	tableNames := []string{
		"accountinfo", "charactermodel", "characters", "charinventory",
		"defaultcharacter", "defaultcharinventory", "defaultspells",
		"hotkeys", "itempattern", "npcitem", "npcs",
		"playerquest", "quests", "spellpattern", "spells", "weaponhotbar",
	}
	for _, name := range tableNames {
		var count int64
		err := db.QueryRow("SELECT COUNT(*) FROM " + name).Scan(&count)
		if err != nil {
			fmt.Printf("  %-25s ERROR: %v\n", name, err)
		} else {
			fmt.Printf("  %-25s %d rows\n", name, count)
		}
	}

	fmt.Printf("\nDatabase written to: %s\n", *dbFile)
}

// shouldSkipLine returns true for lines that should be skipped entirely.
func shouldSkipLine(line string) bool {
	trimmed := strings.TrimSpace(line)

	// Skip empty lines
	if trimmed == "" {
		return true
	}

	// Skip MySQL comments
	if strings.HasPrefix(trimmed, "--") {
		return true
	}

	// Skip MySQL conditional comments (/*!...*/)
	if strings.HasPrefix(trimmed, "/*!")  {
		return true
	}

	// Skip LOCK/UNLOCK TABLES
	if strings.HasPrefix(trimmed, "LOCK TABLES") || strings.HasPrefix(trimmed, "UNLOCK TABLES") {
		return true
	}

	// Skip DROP TABLE
	if strings.HasPrefix(trimmed, "DROP TABLE") {
		return true
	}

	// Skip SET statements
	if strings.HasPrefix(trimmed, "SET ") {
		return true
	}

	// Skip DELIMITER
	if strings.HasPrefix(trimmed, "DELIMITER") {
		return true
	}

	// Skip stored procedures
	if strings.HasPrefix(trimmed, "CREATE DEFINER") {
		return true
	}

	// Skip lines that are part of stored procedures (VALUES inside procedures)
	if strings.HasPrefix(trimmed, "VALUES (") && !strings.Contains(trimmed, "),(") {
		return true
	}

	return false
}

// convertCreateTableLine converts the CREATE TABLE opening line.
func convertCreateTableLine(line string) string {
	// Remove backticks
	line = strings.ReplaceAll(line, "`", "")
	// Already has "CREATE TABLE" - add "IF NOT EXISTS"
	line = strings.Replace(line, "CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ", 1)
	return line
}

// convertColumnLine converts a column or constraint line inside CREATE TABLE.
func convertColumnLine(line string) string {
	trimmed := strings.TrimSpace(line)

	// Remove backticks
	trimmed = strings.ReplaceAll(trimmed, "`", "")

	// Skip MySQL KEY definitions (indexes) but convert UNIQUE KEY to UNIQUE constraint
	if strings.HasPrefix(trimmed, "UNIQUE KEY ") {
		// Extract column name from UNIQUE KEY `name` (`column`)
		re := regexp.MustCompile(`UNIQUE KEY \w+ \((\w+)\)`)
		matches := re.FindStringSubmatch(trimmed)
		if len(matches) > 1 {
			col := matches[1]
			// Will be handled as a post-CREATE INDEX statement; skip for now
			// Actually, we can't easily add UNIQUE to the column def here.
			// Instead, convert to: UNIQUE (column)
			suffix := ","
			if !strings.HasSuffix(trimmed, ",") {
				suffix = ""
			}
			return "  UNIQUE (" + col + ")" + suffix
		}
		return ""
	}
	if strings.HasPrefix(trimmed, "KEY ") {
		return ""
	}

	// Skip CONSTRAINT lines (foreign keys - SQLite handles these differently)
	if strings.HasPrefix(trimmed, "CONSTRAINT ") {
		return ""
	}

	// Convert column types
	trimmed = convertTypes(trimmed)

	// Remove AUTO_INCREMENT from column definitions (SQLite auto-increments INTEGER PRIMARY KEY)
	trimmed = strings.ReplaceAll(trimmed, " AUTO_INCREMENT", "")

	// Remove trailing comma if the next line will be skipped (results in ,) before closing paren)
	// This is handled in cleanCreateTable

	return "  " + trimmed
}

var (
	reIntType    = regexp.MustCompile(`\bint\(\d+\)`)
	reBigintType = regexp.MustCompile(`\bbigint\(\d+\)`)
	reFloatPrec  = regexp.MustCompile(`\bfloat\(\d+,\d+\)`)
	reVarchar    = regexp.MustCompile(`\bvarchar\(\d+\)`)
)

// convertTypes converts MySQL types to SQLite types.
func convertTypes(line string) string {
	line = reIntType.ReplaceAllString(line, "INTEGER")
	line = reBigintType.ReplaceAllString(line, "INTEGER")
	line = reFloatPrec.ReplaceAllString(line, "REAL")
	line = strings.ReplaceAll(line, " float ", " REAL ")
	line = strings.ReplaceAll(line, " float,", " REAL,")
	line = strings.ReplaceAll(line, " datetime ", " DATETIME ")
	line = strings.ReplaceAll(line, " date ", " DATE ")
	line = reVarchar.ReplaceAllString(line, "TEXT")
	line = strings.ReplaceAll(line, " mediumtext ", " TEXT ")
	line = strings.ReplaceAll(line, " binary(66)", " BLOB")

	// Handle "float NOT NULL" and "float DEFAULT" at end of type
	line = regexp.MustCompile(`\bfloat\b`).ReplaceAllString(line, "REAL")

	return line
}

// cleanCreateTable removes trailing commas before closing paren and other issues.
func cleanCreateTable(stmt string) string {
	// Split into lines, remove empty lines, fix trailing comma
	lines := strings.Split(stmt, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}

	// Find the closing paren and remove trailing comma from line before it
	for i := len(cleaned) - 1; i >= 0; i-- {
		if strings.TrimSpace(cleaned[i]) == ");" {
			// Look backward for last non-empty line and remove trailing comma
			for j := i - 1; j >= 0; j-- {
				trimmed := strings.TrimSpace(cleaned[j])
				if trimmed != "" {
					cleaned[j] = strings.TrimSpace(cleaned[j])
					cleaned[j] = strings.TrimRight(cleaned[j], ",")
					cleaned[j] = "  " + cleaned[j]
					break
				}
			}
			break
		}
	}

	return strings.Join(cleaned, "\n")
}

// convertInsert converts a MySQL INSERT statement for SQLite compatibility.
func convertInsert(line string) string {
	// Remove backticks
	line = strings.ReplaceAll(line, "`", "")

	// Handle escaped strings - MySQL uses \' but SQLite uses ''
	// This is tricky with the VALUES data, but for our data it's mostly numeric
	line = strings.ReplaceAll(line, "\\'", "''")

	// Ensure it ends with semicolon
	line = strings.TrimRight(line, " \t\r\n")
	if !strings.HasSuffix(line, ";") {
		line += ";"
	}

	return line
}

// extractTableName extracts the table name from a CREATE TABLE statement.
func extractTableName(stmt string) string {
	re := regexp.MustCompile(`CREATE TABLE (?:IF NOT EXISTS )?(\w+)`)
	matches := re.FindStringSubmatch(stmt)
	if len(matches) > 1 {
		return matches[1]
	}
	return "unknown"
}

// extractInsertTableName extracts the table name from an INSERT statement.
func extractInsertTableName(stmt string) string {
	re := regexp.MustCompile(`INSERT INTO (\w+)`)
	matches := re.FindStringSubmatch(stmt)
	if len(matches) > 1 {
		return strings.ToLower(matches[1])
	}
	return "unknown"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
