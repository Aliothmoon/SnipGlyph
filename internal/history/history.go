// Package history keeps the most recent OCR and translation results in a
// SQLite database with full-text search. Only text is stored.
package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// FileName is the database inside the local app directory.
const FileName = "history.db"

// DefaultLimit is how many entries are kept; older ones are dropped.
const DefaultLimit = 5000

// Kind is what produced an entry.
type Kind string

const (
	OCR        Kind = "ocr"        // screenshot OCR, with the popup or silent
	Screenshot Kind = "screenshot" // screenshot translation
	Selection  Kind = "selection"  // selected text
	Input      Kind = "input"      // typed or pasted text
)

func (k Kind) String() string {
	switch k {
	case OCR:
		return "截图 OCR"
	case Screenshot:
		return "截图翻译"
	case Selection:
		return "划词翻译"
	case Input:
		return "输入翻译"
	}
	return string(k)
}

// Entry is one recorded result.
type Entry struct {
	ID          int64
	Time        time.Time
	Kind        Kind
	Source      string
	Translation string // empty when not translated
	Engine      string // the engine's display name, with Translation
}

// Store is an open history database. It is safe for concurrent use.
type Store struct {
	db    *sql.DB
	limit int
}

// schema creates version 1. The trigram tokenizer matches any substring of
// three or more characters, which suits Chinese and Japanese text that has
// no spaces between words; shorter terms fall back to LIKE (see Search).
// AUTOINCREMENT keeps ids unique across deletions, so a stale id held by an
// open popup never points at a newer entry.
const schema = `
CREATE TABLE entries (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	created     INTEGER NOT NULL, -- Unix milliseconds
	kind        TEXT NOT NULL,
	source      TEXT NOT NULL,
	translation TEXT NOT NULL DEFAULT '',
	engine      TEXT NOT NULL DEFAULT ''
);
CREATE VIRTUAL TABLE entries_fts USING fts5(
	source, translation,
	content='entries', content_rowid='id', tokenize='trigram'
);
CREATE TRIGGER entries_ai AFTER INSERT ON entries BEGIN
	INSERT INTO entries_fts(rowid, source, translation) VALUES (new.id, new.source, new.translation);
END;
CREATE TRIGGER entries_ad AFTER DELETE ON entries BEGIN
	INSERT INTO entries_fts(entries_fts, rowid, source, translation) VALUES ('delete', old.id, old.source, old.translation);
END;
CREATE TRIGGER entries_au AFTER UPDATE ON entries BEGIN
	INSERT INTO entries_fts(entries_fts, rowid, source, translation) VALUES ('delete', old.id, old.source, old.translation);
	INSERT INTO entries_fts(rowid, source, translation) VALUES (new.id, new.source, new.translation);
END;
PRAGMA user_version = 1;
`

// Open opens or creates the database at path, keeping at most limit
// entries (DefaultLimit when limit <= 0).
func Open(path string, limit int) (*Store, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	q := url.Values{}
	for _, p := range []string{"busy_timeout(5000)", "journal_mode(WAL)", "synchronous(NORMAL)"} {
		q.Add("_pragma", p)
	}
	// A plain path (not a file: URI) is used as is; the driver strips the query.
	db, err := sql.Open("sqlite", path+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	// One connection: writes are tiny and serialising them avoids SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	s := &Store{db: db, limit: limit}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open history %s: %w", path, err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	switch version {
	case 0:
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.Exec(schema); err != nil {
			return err
		}
		return tx.Commit()
	case 1:
		return nil
	}
	return fmt.Errorf("database version %d is newer than this program", version)
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Add records e (its ID is ignored), drops entries beyond the limit and
// returns the new entry's id. A zero Time means now.
func (s *Store) Add(ctx context.Context, e Entry) (int64, error) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO entries (created, kind, source, translation, engine) VALUES (?, ?, ?, ?, ?)`,
		e.Time.UnixMilli(), string(e.Kind), e.Source, e.Translation, e.Engine)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM entries WHERE id <= (SELECT id FROM entries ORDER BY id DESC LIMIT 1 OFFSET ?)`,
		s.limit); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// ErrNotFound means the entry was deleted, by the user or by the limit.
var ErrNotFound = errors.New("history entry not found")

// Update replaces the text of entry e.ID, keeping its time and kind.
func (s *Store) Update(ctx context.Context, e Entry) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE entries SET source = ?, translation = ?, engine = ? WHERE id = ?`,
		e.Source, e.Translation, e.Engine, e.ID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// Delete removes one entry.
func (s *Store) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM entries WHERE id = ?`, id)
	return err
}

// Clear removes every entry and compacts the file, so the deleted text does
// not linger in free pages.
func (s *Store) Clear(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM entries`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `VACUUM`)
	return err
}

// Count returns the number of entries.
func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM entries`).Scan(&n)
	return n, err
}

// Search returns up to limit entries, newest first, whose source or
// translation contains every whitespace-separated term of query (case-
// insensitively for Latin letters). An empty query returns the newest.
func (s *Store) Search(ctx context.Context, query string, limit int) ([]Entry, error) {
	where, args := searchFilter(query)
	sqlText := `SELECT id, created, kind, source, translation, engine FROM entries`
	if where != "" {
		sqlText += " WHERE " + where
	}
	sqlText += " ORDER BY id DESC LIMIT ?"
	rows, err := s.db.QueryContext(ctx, sqlText, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var created int64
		var kind string
		if err := rows.Scan(&e.ID, &created, &kind, &e.Source, &e.Translation, &e.Engine); err != nil {
			return nil, err
		}
		e.Time, e.Kind = time.UnixMilli(created), Kind(kind)
		out = append(out, e)
	}
	return out, rows.Err()
}

// searchFilter builds the WHERE clause for Search. Terms of three or more
// characters go through the trigram index as quoted phrases; shorter ones,
// which a trigram index cannot match, are scanned with LIKE. The table is
// capped at a few thousand rows, so the scan stays fast.
func searchFilter(query string) (string, []any) {
	var phrases []string
	var conds []string
	var args []any
	for _, term := range strings.Fields(query) {
		if utf8.RuneCountInString(term) >= 3 {
			phrases = append(phrases, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
			continue
		}
		pat := "%" + likeEscaper.Replace(term) + "%"
		conds = append(conds, `(source LIKE ? ESCAPE '\' OR translation LIKE ? ESCAPE '\')`)
		args = append(args, pat, pat)
	}
	if len(phrases) > 0 {
		conds = append([]string{`id IN (SELECT rowid FROM entries_fts WHERE entries_fts MATCH ?)`}, conds...)
		args = append([]any{strings.Join(phrases, " ")}, args...)
	}
	return strings.Join(conds, " AND "), args
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
