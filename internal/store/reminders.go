package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// Reminder is a message to deliver to a user at a later time.
type Reminder struct {
	ID     int64
	UserID snowflake.ID
	// GuildID is the server the reminder was set in (0 for DMs).
	GuildID snowflake.ID
	// ChannelID is where to post; 0 means DM the user.
	ChannelID snowflake.ID
	// SourceChannelID is where the reminder was set, used as a fallback when
	// the user's DMs are closed.
	SourceChannelID snowflake.ID
	Message         string
	DueAt           time.Time
	CreatedAt       time.Time
	Attempts        int
}

const reminderColumns = `id, user_id, guild_id, channel_id, source_channel_id, message, due_at, created_at, attempts`

func scanReminder(scan func(...any) error) (Reminder, error) {
	var r Reminder
	var user, guild, channel, source, due, created int64
	err := scan(&r.ID, &user, &guild, &channel, &source, &r.Message, &due, &created, &r.Attempts)
	r.UserID, r.GuildID, r.ChannelID, r.SourceChannelID = snowflake.ID(user), snowflake.ID(guild), snowflake.ID(channel), snowflake.ID(source)
	r.DueAt, r.CreatedAt = time.Unix(due, 0), time.Unix(created, 0)
	return r, err
}

func (s *Store) queryReminders(ctx context.Context, where string, args ...any) ([]Reminder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+reminderColumns+` FROM reminders `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Reminder
	for rows.Next() {
		r, err := scanReminder(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddReminder stores a reminder and returns it with its ID set.
func (s *Store) AddReminder(ctx context.Context, r Reminder) (Reminder, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO reminders (user_id, guild_id, channel_id, source_channel_id, message, due_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		int64(r.UserID), int64(r.GuildID), int64(r.ChannelID), int64(r.SourceChannelID), r.Message, r.DueAt.Unix(), r.CreatedAt.Unix())
	if err != nil {
		return r, err
	}
	r.ID, err = res.LastInsertId()
	return r, err
}

// UserReminders returns a user's pending reminders, soonest first.
func (s *Store) UserReminders(ctx context.Context, userID snowflake.ID) ([]Reminder, error) {
	return s.queryReminders(ctx, `WHERE user_id = ? ORDER BY due_at, id`, int64(userID))
}

// DueReminders returns reminders due at or before t, oldest first.
func (s *Store) DueReminders(ctx context.Context, t time.Time) ([]Reminder, error) {
	return s.queryReminders(ctx, `WHERE due_at <= ? ORDER BY due_at, id LIMIT 100`, t.Unix())
}

// NextReminderDue returns when the next reminder is due.
func (s *Store) NextReminderDue(ctx context.Context) (time.Time, bool, error) {
	var due sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MIN(due_at) FROM reminders`).Scan(&due); err != nil {
		return time.Time{}, false, err
	}
	if !due.Valid {
		return time.Time{}, false, nil
	}
	return time.Unix(due.Int64, 0), true, nil
}

// DeleteReminder removes a reminder. If userID is non-zero, the reminder
// must belong to that user; sql.ErrNoRows is returned otherwise.
func (s *Store) DeleteReminder(ctx context.Context, id int64, userID snowflake.ID) error {
	if userID == 0 {
		res, err := s.db.ExecContext(ctx, `DELETE FROM reminders WHERE id = ?`, id)
		return affected(res, err)
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM reminders WHERE id = ? AND user_id = ?`, id, int64(userID))
	return affected(res, err)
}

// RetryReminder records a failed delivery and pushes the reminder back.
func (s *Store) RetryReminder(ctx context.Context, id int64, next time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE reminders SET attempts = attempts + 1, due_at = ? WHERE id = ?`, next.Unix(), id)
	return err
}

// UserTimezone returns a user's saved IANA time zone, if any.
func (s *Store) UserTimezone(ctx context.Context, userID snowflake.ID) (string, bool, error) {
	var tz sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT timezone FROM user_settings WHERE user_id = ?`, int64(userID)).Scan(&tz)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return tz.String, tz.Valid && tz.String != "", nil
}

// SetUserTimezone saves (or with "" clears) a user's time zone.
func (s *Store) SetUserTimezone(ctx context.Context, userID snowflake.ID, tz string) error {
	var value any
	if tz != "" {
		value = tz
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO user_settings (user_id, timezone) VALUES (?, ?)
		ON CONFLICT(user_id) DO UPDATE SET timezone = excluded.timezone`, int64(userID), value)
	return err
}
