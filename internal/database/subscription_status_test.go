package database

import (
	"testing"

	"github.com/ppxb/miyabi/internal/ent/subscription"
)

func TestLegacyAddedSubscriptionMigratesToWaiting(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if store != nil {
			_ = store.Close()
		}
	})

	// Simulate a legacy database row carrying the retired "added" status by
	// writing the enum value directly below ent's level.
	if _, err := store.db.Exec(`INSERT INTO subscriptions
		(created_at, updated_at, kind, target_id, code, title, cover, release_date,
		 auto_download, status, cursor, hash, checks)
		VALUES (datetime('now'), datetime('now'), 'movie', 'legacy-1', 'ABP-123', '', '', '',
		 1, 'added', '', 'legacyhash', 1)`); err != nil {
		t.Fatal(err)
	}
	legacy := store.Client.Subscription.Query().
		Where(subscription.TargetIDEQ("legacy-1")).OnlyX(t.Context())

	// Reopen runs the schema migration and the status data migration.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}

	reloaded := store.Client.Subscription.GetX(t.Context(), legacy.ID)
	if reloaded.Status != subscription.StatusWaiting {
		t.Fatalf("legacy added row must resume waiting, got %q", reloaded.Status)
	}
	if reloaded.Hash != "legacyhash" {
		t.Fatalf("the recorded magnet hash must survive migration, got %q", reloaded.Hash)
	}
}
