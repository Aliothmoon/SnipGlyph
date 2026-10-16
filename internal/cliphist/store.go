package cliphist

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// FileName is the database inside the local app directory.
const FileName = "clipboard.db"

// DefaultTab collects new copies and is the only tab trimmed to a limit.
const DefaultTab int64 = 1

// DefaultLimit is how many unpinned items the default tab keeps.
const DefaultLimit = 5000

// Tab is a named list of items.
type Tab struct {
	ID   int64
	Name string
}

// Summary is an item as listed: its text cut short and no image data.
type Summary struct {
	ID       int64
	Kind     Kind
	Preview  string // the start of the text
	Width    int
	Height   int
	Source   string
	Pinned   bool
	Rich     bool // has HTML or RTF besides the text
	Count    int  // of paths, for Files
	Used     time.Time
	HasThumb bool
}

var (
	// ErrNotFound means the item or tab no longer exists.
	ErrNotFound = errors.New("not found")
	// ErrTabExists means another tab has that name.
	ErrTabExists = errors.New("a tab with that name already exists")
)

// Store is an open clipboard database. It is safe for concurrent use.
type Store struct {
	db    *sql.DB
	mu    sync.Mutex
	limit int
}

const schema = `
CREATE TABLE tabs (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	name     TEXT NOT NULL UNIQUE,
	position INTEGER NOT NULL
);
INSERT INTO tabs (id, name, position) VALUES (1, '剪贴板', 0);
CREATE TABLE items (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	tab     INTEGER NOT NULL REFERENCES tabs(id) ON DELETE CASCADE,
	kind    TEXT NOT NULL,
	text    TEXT NOT NULL DEFAULT '',
	html    BLOB,
	rtf     BLOB,
	png     BLOB,
	thumb   BLOB,
	width   INTEGER NOT NULL DEFAULT 0,
	height  INTEGER NOT NULL DEFAULT 0,
	source  TEXT NOT NULL DEFAULT '',
	hash    BLOB NOT NULL,
	pinned  INTEGER NOT NULL DEFAULT 0,
	created INTEGER NOT NULL, -- Unix milliseconds
	used    INTEGER NOT NULL  -- last copied: the list order
);
CREATE INDEX items_order ON items(tab, pinned DESC, used DESC);
CREATE INDEX items_hash ON items(tab, hash);
CREATE VIRTUAL TABLE items_fts USING fts5(
	text, content='items', content_rowid='id', tokenize='trigram'
);
CREATE TRIGGER items_ai AFTER INSERT ON items BEGIN
	INSERT INTO items_fts(rowid, text) VALUES (new.id, new.text);
END;
CREATE TRIGGER items_ad AFTER DELETE ON items BEGIN
	INSERT INTO items_fts(items_fts, rowid, text) VALUES ('delete', old.id, old.text);
END;
CREATE TRIGGER items_au AFTER UPDATE OF text ON items BEGIN
	INSERT INTO items_fts(items_fts, rowid, text) VALUES ('delete', old.id, old.text);
	INSERT INTO items_fts(rowid, text) VALUES (new.id, new.text);
END;
PRAGMA user_version = 1;
`

// Open opens or creates the database at path; the default tab keeps at
// most limit unpinned items (DefaultLimit when limit <= 0).
func Open(path string, limit int) (*Store, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	q := url.Values{}
	for _, p := range []string{"busy_timeout(5000)", "journal_mode(WAL)", "synchronous(NORMAL)", "foreign_keys(1)"} {
		q.Add("_pragma", p)
	}
	db, err := sql.Open("sqlite", path+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, limit: limit}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open clipboard history %s: %w", path, err)
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

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// nextUsed is the order stamp for an item going to the top: the time,
// nudged past the newest stamp so two copies in one millisecond still
// keep their order.
func nextUsed(ctx context.Context, q querier) int64 {
	t := time.Now().UnixMilli()
	var newest sql.NullInt64
	q.QueryRowContext(ctx, `SELECT max(used) FROM items`).Scan(&newest)
	return max(t, newest.Int64+1)
}

// Record files a new copy in the default tab. When the tab already holds
// the same content, that item moves to the top and takes the new formats
// instead. Unpinned items beyond the limit are dropped.
func (s *Store) Record(ctx context.Context, it Item) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM items WHERE tab = ? AND hash = ? LIMIT 1`, DefaultTab, it.Hash).Scan(&id)
	switch {
	case err == nil:
		_, err = tx.ExecContext(ctx,
			`UPDATE items SET html = ?, rtf = ?, png = ?, thumb = ?, width = ?, height = ?, source = ?, used = ? WHERE id = ?`,
			it.HTML, it.RTF, it.PNG, it.Thumb, it.Width, it.Height, it.Source, nextUsed(ctx, tx), id)
	case errors.Is(err, sql.ErrNoRows):
		id, err = insert(ctx, tx, DefaultTab, it)
	}
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM items WHERE id IN (
		SELECT id FROM items WHERE tab = ? AND pinned = 0 ORDER BY used DESC, id DESC LIMIT -1 OFFSET ?)`,
		DefaultTab, s.currentLimit()); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) currentLimit() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.limit
}

func insert(ctx context.Context, tx *sql.Tx, tab int64, it Item) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO items
		(tab, kind, text, html, rtf, png, thumb, width, height, source, hash, pinned, created, used)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tab, string(it.Kind), it.Text, it.HTML, it.RTF, it.PNG, it.Thumb, it.Width, it.Height, it.Source, it.Hash, it.Pinned,
		time.Now().UnixMilli(), nextUsed(ctx, tx))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// List returns up to limit items of a tab, pinned first, then the most
// recently copied, keeping those whose text contains every term of query.
func (s *Store) List(ctx context.Context, tab int64, query string, limit int) ([]Summary, error) {
	where, args := searchFilter(query)
	q := `SELECT id, kind, substr(text, 1, 400), width, height, source, pinned,
		html IS NOT NULL OR rtf IS NOT NULL, used, thumb IS NOT NULL,
		CASE kind WHEN 'files' THEN length(text) - length(replace(text, char(10), '')) + 1 ELSE 0 END
		FROM items WHERE tab = ?`
	if where != "" {
		q += " AND " + where
	}
	q += " ORDER BY pinned DESC, used DESC, id DESC LIMIT ?"
	args = append(append([]any{tab}, args...), limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var sm Summary
		var kind string
		var used int64
		if err := rows.Scan(&sm.ID, &kind, &sm.Preview, &sm.Width, &sm.Height, &sm.Source, &sm.Pinned, &sm.Rich, &used, &sm.HasThumb, &sm.Count); err != nil {
			return nil, err
		}
		sm.Kind, sm.Used = Kind(kind), time.UnixMilli(used)
		out = append(out, sm)
	}
	return out, rows.Err()
}

// searchFilter matches terms of three or more characters through the
// trigram index and shorter ones with LIKE, which the index cannot serve.
func searchFilter(query string) (string, []any) {
	var phrases, conds []string
	var args []any
	for _, term := range strings.Fields(query) {
		if utf8.RuneCountInString(term) >= 3 {
			phrases = append(phrases, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
			continue
		}
		conds = append(conds, `text LIKE ? ESCAPE '\'`)
		args = append(args, "%"+likeEscaper.Replace(term)+"%")
	}
	if len(phrases) > 0 {
		conds = append([]string{`id IN (SELECT rowid FROM items_fts WHERE items_fts MATCH ?)`}, conds...)
		args = append([]any{strings.Join(phrases, " ")}, args...)
	}
	return strings.Join(conds, " AND "), args
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// Get returns an item with all its data.
func (s *Store) Get(ctx context.Context, id int64) (Item, error) {
	var it Item
	var kind string
	var used int64
	err := s.db.QueryRowContext(ctx, `SELECT id, tab, kind, text, html, rtf, png, thumb, width, height, source, pinned, hash, used
		FROM items WHERE id = ?`, id).Scan(&it.ID, &it.Tab, &kind, &it.Text, &it.HTML, &it.RTF, &it.PNG, &it.Thumb,
		&it.Width, &it.Height, &it.Source, &it.Pinned, &it.Hash, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return it, ErrNotFound
	}
	it.Kind, it.Used = Kind(kind), time.UnixMilli(used)
	return it, err
}

// Thumb returns an image item's thumbnail PNG.
func (s *Store) Thumb(ctx context.Context, id int64) ([]byte, error) {
	var b []byte
	err := s.db.QueryRowContext(ctx, `SELECT thumb FROM items WHERE id = ?`, id).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return b, err
}

func (s *Store) exec(ctx context.Context, q string, args ...any) error {
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// SetPinned pins an item to the top of its tab, where the limit does not
// reach it, or unpins it.
func (s *Store) SetPinned(ctx context.Context, id int64, pinned bool) error {
	return s.exec(ctx, `UPDATE items SET pinned = ? WHERE id = ?`, pinned, id)
}

// SetText replaces a text item's text. Its HTML and RTF no longer match
// and are dropped.
func (s *Store) SetText(ctx context.Context, id int64, text string) error {
	return s.exec(ctx, `UPDATE items SET text = ?, html = NULL, rtf = NULL, hash = ? WHERE id = ? AND kind = ?`,
		text, hash(Text, []byte(text)), id, string(Text))
}

// Delete removes an item.
func (s *Store) Delete(ctx context.Context, id int64) error {
	return s.exec(ctx, `DELETE FROM items WHERE id = ?`, id)
}

// Touch moves an item to the top of its tab, as when it is used again.
func (s *Store) Touch(ctx context.Context, id int64) error {
	return s.exec(ctx, `UPDATE items SET used = ? WHERE id = ?`, nextUsed(ctx, s.db), id)
}

// SetLimit changes how many unpinned items the default tab keeps, from
// the next Record on.
func (s *Store) SetLimit(limit int) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	s.mu.Lock()
	s.limit = limit
	s.mu.Unlock()
}

// MoveTo puts an item in another tab, at the top.
func (s *Store) MoveTo(ctx context.Context, id, tab int64) error {
	return s.exec(ctx, `UPDATE items SET tab = ?, used = ? WHERE id = ?`, tab, nextUsed(ctx, s.db), id)
}

// CopyTo adds a copy of an item to another tab, at the top.
func (s *Store) CopyTo(ctx context.Context, id, tab int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO items
		(tab, kind, text, html, rtf, png, thumb, width, height, source, hash, pinned, created, used)
		SELECT ?, kind, text, html, rtf, png, thumb, width, height, source, hash, 0, ?, ? FROM items WHERE id = ?`,
		tab, time.Now().UnixMilli(), nextUsed(ctx, s.db), id)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrNotFound
	}
	return res.LastInsertId()
}

// Count returns how many items a tab holds.
func (s *Store) Count(ctx context.Context, tab int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM items WHERE tab = ?`, tab).Scan(&n)
	return n, err
}

// Tabs lists the tabs in order; the default tab comes first.
func (s *Store) Tabs(ctx context.Context) ([]Tab, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name FROM tabs ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tab
	for rows.Next() {
		var t Tab
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func checkTabName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("tab name is empty")
	}
	return name, nil
}

func tabErr(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return ErrTabExists
	}
	return err
}

// AddTab appends a tab.
func (s *Store) AddTab(ctx context.Context, name string) (Tab, error) {
	name, err := checkTabName(name)
	if err != nil {
		return Tab{}, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO tabs (name, position) VALUES (?, (SELECT coalesce(max(position), 0) + 1 FROM tabs))`, name)
	if err != nil {
		return Tab{}, tabErr(err)
	}
	id, err := res.LastInsertId()
	return Tab{ID: id, Name: name}, err
}

// RenameTab renames a tab.
func (s *Store) RenameTab(ctx context.Context, id int64, name string) error {
	name, err := checkTabName(name)
	if err != nil {
		return err
	}
	return tabErr(s.exec(ctx, `UPDATE tabs SET name = ? WHERE id = ?`, name, id))
}

// DeleteTab removes a tab and its items. The default tab cannot go.
func (s *Store) DeleteTab(ctx context.Context, id int64) error {
	if id == DefaultTab {
		return errors.New("the default tab cannot be deleted")
	}
	return s.exec(ctx, `DELETE FROM tabs WHERE id = ?`, id)
}
