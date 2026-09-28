// Package store persists per-guild settings in SQLite.
package store

import (
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS guild_settings (
	guild_id INTEGER PRIMARY KEY,
	prefix   TEXT
);
CREATE TABLE IF NOT EXISTS aliases (
	guild_id   INTEGER NOT NULL,
	name       TEXT    NOT NULL,
	command    TEXT    NOT NULL,
	creator_id INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	PRIMARY KEY (guild_id, name)
);
CREATE TABLE IF NOT EXISTS command_stats (
	name  TEXT PRIMARY KEY,
	count INTEGER NOT NULL
);`

type Store struct {
	db *sql.DB

	mu       sync.RWMutex
	prefixes map[snowflake.ID]string
	aliases  map[snowflake.ID]map[string]Alias
}

// Alias maps a prefix-only command name to a command line.
type Alias struct {
	Name      string
	Command   string
	CreatorID snowflake.ID
	CreatedAt time.Time
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	s := &Store{db: db, prefixes: map[snowflake.ID]string{}, aliases: map[snowflake.ID]map[string]Alias{}}
	if err := s.loadPrefixes(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.loadAliases(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) loadPrefixes() error {
	rows, err := s.db.Query(`SELECT guild_id, prefix FROM guild_settings WHERE prefix IS NOT NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var prefix string
		if err := rows.Scan(&id, &prefix); err != nil {
			return err
		}
		s.prefixes[snowflake.ID(id)] = prefix
	}
	return rows.Err()
}

// Prefix returns the guild's prefix override, if any.
func (s *Store) Prefix(guildID snowflake.ID) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.prefixes[guildID]
	return p, ok
}

// SetPrefix sets the guild's prefix override. An empty prefix removes it.
func (s *Store) SetPrefix(guildID snowflake.ID, prefix string) error {
	var value any
	if prefix != "" {
		value = prefix
	}
	_, err := s.db.Exec(`INSERT INTO guild_settings (guild_id, prefix) VALUES (?, ?)
		ON CONFLICT(guild_id) DO UPDATE SET prefix = excluded.prefix`, int64(guildID), value)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if prefix == "" {
		delete(s.prefixes, guildID)
	} else {
		s.prefixes[guildID] = prefix
	}
	return nil
}

func (s *Store) loadAliases() error {
	rows, err := s.db.Query(`SELECT guild_id, name, command, creator_id, created_at FROM aliases`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var gid, creator, created int64
		var a Alias
		if err := rows.Scan(&gid, &a.Name, &a.Command, &creator, &created); err != nil {
			return err
		}
		a.CreatorID = snowflake.ID(creator)
		a.CreatedAt = time.Unix(created, 0)
		if s.aliases[snowflake.ID(gid)] == nil {
			s.aliases[snowflake.ID(gid)] = map[string]Alias{}
		}
		s.aliases[snowflake.ID(gid)][a.Name] = a
	}
	return rows.Err()
}

// Alias looks up a guild alias by name.
func (s *Store) Alias(guildID snowflake.ID, name string) (Alias, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.aliases[guildID][name]
	return a, ok
}

// Aliases returns a guild's aliases sorted by name.
func (s *Store) Aliases(guildID snowflake.ID) []Alias {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Alias, 0, len(s.aliases[guildID]))
	for _, a := range s.aliases[guildID] {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// SetAlias creates or replaces a guild alias.
func (s *Store) SetAlias(guildID snowflake.ID, a Alias) error {
	_, err := s.db.Exec(`INSERT INTO aliases (guild_id, name, command, creator_id, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(guild_id, name) DO UPDATE SET command = excluded.command, creator_id = excluded.creator_id, created_at = excluded.created_at`,
		int64(guildID), a.Name, a.Command, int64(a.CreatorID), a.CreatedAt.Unix())
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.aliases[guildID] == nil {
		s.aliases[guildID] = map[string]Alias{}
	}
	s.aliases[guildID][a.Name] = a
	return nil
}

// DeleteAlias removes a guild alias, reporting whether it existed.
func (s *Store) DeleteAlias(guildID snowflake.ID, name string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM aliases WHERE guild_id = ? AND name = ?`, int64(guildID), name)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.aliases[guildID], name)
	return n > 0, nil
}

// CommandCount is a command's all-time usage count.
type CommandCount struct {
	Name  string
	Count int64
}

// IncrementCommand bumps a command's all-time usage count.
func (s *Store) IncrementCommand(name string) error {
	_, err := s.db.Exec(`INSERT INTO command_stats (name, count) VALUES (?, 1)
		ON CONFLICT(name) DO UPDATE SET count = count + 1`, name)
	return err
}

// CommandCounts returns all-time usage counts, most used first.
func (s *Store) CommandCounts() ([]CommandCount, error) {
	rows, err := s.db.Query(`SELECT name, count FROM command_stats ORDER BY count DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommandCount
	for rows.Next() {
		var c CommandCount
		if err := rows.Scan(&c.Name, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
