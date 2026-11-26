package database

import (
	"context"
	"database/sql"
	"log/slog"
)

// migrateLegacySubscriptionStatus rewrites the retired "added" subscription
// status. No cloud drive submits magnets on the user's behalf anymore, so a
// legacy row keeps its recorded hash but resumes the daily waiting loop.
func migrateLegacySubscriptionStatus(ctx context.Context, db *sql.DB) error {
	result, err := db.ExecContext(ctx, `UPDATE subscriptions SET status = 'waiting' WHERE status = 'added'`)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err == nil && affected > 0 {
		slog.InfoContext(ctx, "migrated legacy subscription status", "rows", affected)
	}
	return nil
}
