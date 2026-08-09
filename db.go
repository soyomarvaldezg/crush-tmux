package main

// Read-only DB watermarking. The DSN forces mode=ro so this binary can NEVER
// write to a crush database: the driver itself rejects any write, which makes
// "read-only forever" a structural guarantee rather than a convention.

import (
	"database/sql"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// dbPathFor returns the per-project crush DB for a working directory.
func dbPathFor(cwd string) string {
	return filepath.Join(cwd, ".crush", "crush.db")
}

// openRO opens a crush DB strictly read-only. mode=ro makes writes fail at the
// driver level; _query_only=true makes SQLite itself refuse any write/pragma.
func openRO(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?mode=ro&_query_only=true"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // serialize; we are a read-only poller
	return db, nil
}

// sessionActivity is the per-session watermark snapshot.
type sessionActivity struct {
	SessionID string
	Title     string
	MaxMsg    int64 // MAX(messages.created_at), ms; 0 if no messages
	UpdatedAt int64 // sessions.updated_at, ms
}

// readActivity reads the watermark for every session in one project DB.
// One indexed query; safe to run every few seconds alongside a live crush.
func readActivity(db *sql.DB) ([]sessionActivity, error) {
	rows, err := db.Query(`
		SELECT s.id, s.title, COALESCE(MAX(m.created_at), 0), s.updated_at
		FROM sessions s
		LEFT JOIN messages m ON m.session_id = s.id
		GROUP BY s.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []sessionActivity
	for rows.Next() {
		var a sessionActivity
		if err := rows.Scan(&a.SessionID, &a.Title, &a.MaxMsg, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// latestWatermark returns the highest message watermark across all sessions in
// the project DB for this cwd. Returns 0 (and no error) if the DB is absent.
func latestWatermark(cwd string) (int64, error) {
	db, err := openRO(dbPathFor(cwd))
	if err != nil {
		return 0, nil // no DB yet: treat as no activity
	}
	defer db.Close()
	var max int64
	err = db.QueryRow(`SELECT COALESCE(MAX(created_at), 0) FROM messages`).Scan(&max)
	if err != nil {
		return 0, nil // empty/unreadable DB is not fatal
	}
	return max, nil
}
