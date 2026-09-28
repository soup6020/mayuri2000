package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// ErrFeedExists is returned when a channel already has a feed with the name.
var ErrFeedExists = errors.New("a feed with that name already exists in the channel")

// Feed is an RSS/Atom subscription that posts to a channel.
type Feed struct {
	ID        int64
	GuildID   snowflake.ID
	ChannelID snowflake.ID
	Name      string
	URL       string
	Template  string
	Embed     bool
	// Seen holds the IDs of recently seen items, newest first.
	Seen         []string
	ETag         string
	LastModified string
	LastError    string
	LastPost     time.Time
	CreatorID    snowflake.ID
	CreatedAt    time.Time
}

const feedColumns = `id, guild_id, channel_id, name, url, template, embed, seen, etag, last_modified, last_error, last_post, creator_id, created_at`

func scanFeed(scan func(...any) error) (Feed, error) {
	var (
		f                       Feed
		guild, channel, creator int64
		lastPost, created       int64
		embed                   int
		seen                    string
	)
	err := scan(&f.ID, &guild, &channel, &f.Name, &f.URL, &f.Template, &embed, &seen,
		&f.ETag, &f.LastModified, &f.LastError, &lastPost, &creator, &created)
	if err != nil {
		return f, err
	}
	f.GuildID, f.ChannelID, f.CreatorID = snowflake.ID(guild), snowflake.ID(channel), snowflake.ID(creator)
	f.Embed = embed != 0
	if lastPost > 0 {
		f.LastPost = time.Unix(lastPost, 0)
	}
	f.CreatedAt = time.Unix(created, 0)
	_ = json.Unmarshal([]byte(seen), &f.Seen)
	return f, nil
}

func (s *Store) queryFeeds(ctx context.Context, where string, args ...any) ([]Feed, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+feedColumns+` FROM rss_feeds `+where+` ORDER BY guild_id, channel_id, name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Feed
	for rows.Next() {
		f, err := scanFeed(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// Feeds returns every feed.
func (s *Store) Feeds(ctx context.Context) ([]Feed, error) {
	return s.queryFeeds(ctx, "")
}

// GuildFeeds returns a guild's feeds.
func (s *Store) GuildFeeds(ctx context.Context, guildID snowflake.ID) ([]Feed, error) {
	return s.queryFeeds(ctx, "WHERE guild_id = ?", int64(guildID))
}

// Feed returns a feed by ID.
func (s *Store) Feed(ctx context.Context, id int64) (Feed, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+feedColumns+` FROM rss_feeds WHERE id = ?`, id)
	return scanFeed(row.Scan)
}

func encodeSeen(seen []string) string {
	b, _ := json.Marshal(seen)
	return string(b)
}

// AddFeed stores a new feed and returns it with its ID set.
func (s *Store) AddFeed(ctx context.Context, f Feed) (Feed, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO rss_feeds
		(guild_id, channel_id, name, url, template, embed, seen, etag, last_modified, creator_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		int64(f.GuildID), int64(f.ChannelID), f.Name, f.URL, f.Template, boolInt(f.Embed), encodeSeen(f.Seen),
		f.ETag, f.LastModified, int64(f.CreatorID), f.CreatedAt.Unix())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return f, ErrFeedExists
		}
		return f, err
	}
	f.ID, err = res.LastInsertId()
	return f, err
}

// UpdateFeedState records the result of polling a feed.
func (s *Store) UpdateFeedState(ctx context.Context, f Feed) error {
	var lastPost int64
	if !f.LastPost.IsZero() {
		lastPost = f.LastPost.Unix()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE rss_feeds SET seen = ?, etag = ?, last_modified = ?, last_error = ?, last_post = ? WHERE id = ?`,
		encodeSeen(f.Seen), f.ETag, f.LastModified, f.LastError, lastPost, f.ID)
	return err
}

// UpdateFeedSettings changes a feed's template and embed mode.
func (s *Store) UpdateFeedSettings(ctx context.Context, id int64, template string, embed bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE rss_feeds SET template = ?, embed = ? WHERE id = ?`, template, boolInt(embed), id)
	return affected(res, err)
}

// DeleteFeed removes a feed.
func (s *Store) DeleteFeed(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM rss_feeds WHERE id = ?`, id)
	return affected(res, err)
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
