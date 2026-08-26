package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/server/model"
)

// prefColumns is the column list shared by every preferences read.
const prefColumns = `user_id, show_email, show_activity, show_orgs, searchable,
	email_security, email_org, email_product, theme, font_size, reduce_motion,
	date_format, time_format, updated_at`

// Preferences reads a user's saved preferences.
//
// PART 34 creates the row lazily, so a user who has never changed a
// setting simply has no row. That is not an error: the documented
// defaults are returned instead, which keeps every caller from having
// to special-case a missing row.
func (s *Store) Preferences(ctx context.Context, userID int64) (model.Preferences, error) {
	var (
		p                                                       model.Preferences
		showEmail, showActivity, showOrgs, searchable, security int
		org, product, reduceMotion                              int
		updated                                                 any
	)

	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			return row.Scan(&p.UserID, &showEmail, &showActivity, &showOrgs,
				&searchable, &security, &org, &product, &p.Theme, &p.FontSize,
				&reduceMotion, &p.DateFormat, &p.TimeFormat, &updated)
		},
		`SELECT `+prefColumns+` FROM user_preferences WHERE user_id = ?`, userID)
	if err != nil {
		if errors.Is(notFound(err), ErrNotFound) {
			return model.DefaultPreferences(userID), nil
		}
		return model.Preferences{}, err
	}

	p.ShowEmail = showEmail != 0
	p.ShowActivity = showActivity != 0
	p.ShowOrgs = showOrgs != 0
	p.Searchable = searchable != 0
	p.EmailSecurity = security != 0
	p.EmailOrg = org != 0
	p.EmailProduct = product != 0
	p.ReduceMotion = reduceMotion != 0
	p.UpdatedAt = database.ScanTime(updated)
	return p, nil
}

// SavePreferences writes a user's preferences, creating the row on
// first save. The update path runs first so an existing row is never
// duplicated, which avoids depending on driver-specific upsert syntax
// that MSSQL and MongoDB do not share with the SQL drivers.
func (s *Store) SavePreferences(ctx context.Context, p model.Preferences) error {
	ts := database.FormatTime(now())

	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE user_preferences SET show_email = ?, show_activity = ?,
			show_orgs = ?, searchable = ?, email_security = ?, email_org = ?,
			email_product = ?, theme = ?, font_size = ?, reduce_motion = ?,
			date_format = ?, time_format = ?, updated_at = ?
		 WHERE user_id = ?`,
		boolInt(p.ShowEmail), boolInt(p.ShowActivity), boolInt(p.ShowOrgs),
		boolInt(p.Searchable), boolInt(p.EmailSecurity), boolInt(p.EmailOrg),
		boolInt(p.EmailProduct), p.Theme, p.FontSize, boolInt(p.ReduceMotion),
		p.DateFormat, p.TimeFormat, ts, p.UserID)
	if err != nil {
		return err
	}
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n > 0 {
		return nil
	}

	_, err = database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`INSERT INTO user_preferences (user_id, show_email, show_activity,
			show_orgs, searchable, email_security, email_org, email_product,
			theme, font_size, reduce_motion, date_format, time_format, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.UserID, boolInt(p.ShowEmail), boolInt(p.ShowActivity),
		boolInt(p.ShowOrgs), boolInt(p.Searchable), boolInt(p.EmailSecurity),
		boolInt(p.EmailOrg), boolInt(p.EmailProduct), p.Theme, p.FontSize,
		boolInt(p.ReduceMotion), p.DateFormat, p.TimeFormat, ts)
	return err
}
