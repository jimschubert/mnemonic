package main

type SQLiteCmd struct {
	Import SQLiteImportCmd `cmd:"import" help:"Import entries from a YAML store to a SQLite store"`
	Export SQLiteExportCmd `cmd:"export" help:"Export entries from a SQLite store to a YAML store"`
}
