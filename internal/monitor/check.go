package monitor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/subscription"
	"github.com/ppxb/miyabi/internal/magnet"
)

const (
	retryInterval     = time.Hour
	movieBatchSize    = 10
	actorBatchSize    = 5
	requestGap        = 2 * time.Second
	actorFeedPageSize = 40
)

// Check drains subscriptions due at the start of this run in small, alternating
// movie and actor pages. The scheduler runs checks serially, with paced requests.
func (service *Service) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cfg := service.config(ctx)
	picker := magnet.NewPicker(cfg.Preferences)
	requests := 0
	pace := func() error {
		if requests > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(requestGap):
			}
		}
		requests++
		return nil
	}

	cutoff := time.Now()
	var hits []string
	groups := []struct {
		kind   subscription.Kind
		status subscription.Status
		limit  int
		after  *ent.Subscription
		done   bool
	}{
		{kind: subscription.KindMovie, status: subscription.StatusWaiting, limit: movieBatchSize},
		{kind: subscription.KindActor, status: subscription.StatusActive, limit: actorBatchSize},
	}
	var checkErrors []error
	for {
		more := false
		for i := range groups {
			group := &groups[i]
			if group.done {
				continue
			}
			records, err := service.dueSubscriptions(ctx, group.kind, group.status, cutoff, group.after, group.limit)
			if err != nil {
				return errors.Join(append(checkErrors, err)...)
			}
			group.done = len(records) < group.limit
			more = more || !group.done
			if len(records) > 0 {
				group.after = records[len(records)-1]
			}
			for _, record := range records {
				if err := pace(); err != nil {
					return err
				}
				if group.kind == subscription.KindMovie {
					err = service.checkMovie(ctx, record, picker, cfg.CheckTime, &hits)
				} else {
					err = service.checkActor(ctx, record, cfg.CheckTime)
				}
				if err != nil {
					if ctx.Err() != nil {
						return err
					}
					checkErrors = append(checkErrors, err)
				}
			}
		}
		if !more {
			break
		}
	}
	service.notifyHits(ctx, cfg, hits, len(checkErrors))
	return errors.Join(checkErrors...)
}

func (service *Service) dueSubscriptions(ctx context.Context, kind subscription.Kind, status subscription.Status, cutoff time.Time, after *ent.Subscription, limit int) ([]*ent.Subscription, error) {
	query := service.database.Subscription.Query().
		Where(subscription.KindEQ(kind), subscription.StatusEQ(status), subscription.NextCheckAtLTE(cutoff))
	if after != nil {
		// Updates remove rows from the due set. A time/ID cursor avoids skipping
		// rows like offset pagination, or rechecking rows whose update failed.
		query.Where(subscription.Or(
			subscription.NextCheckAtGT(*after.NextCheckAt),
			subscription.And(subscription.NextCheckAtEQ(*after.NextCheckAt), subscription.IDGT(after.ID)),
		))
	}
	records, err := query.Order(ent.Asc(subscription.FieldNextCheckAt), ent.Asc(subscription.FieldID)).Limit(limit).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load due %s subscriptions: %w", kind, err)
	}
	return records, nil
}

func (service *Service) checkMovie(ctx context.Context, record *ent.Subscription, picker *magnet.Picker, checkTime string, hits *[]string) error {
	now := time.Now()
	magnets, err := service.discover.CatalogueMagnets(ctx, record.TargetID)
	if err != nil {
		return service.deferCheck(ctx, record, now, domain.E(domain.KindUpstream, "查询磁力失败："+domain.PublicMessage(err), err))
	}
	best, found := picker.Pick(magnets)
	if !found {
		next, stale := nextMovieCheck(now, record.ReleaseDate, record.CreatedAt, checkTime)
		update := record.Update().SetLastCheckedAt(now).AddChecks(1).ClearError()
		if stale {
			update.SetStatus(subscription.StatusStale).ClearNextCheckAt()
		} else {
			update.SetNextCheckAt(next)
		}
		if err := update.Exec(ctx); err != nil && !ent.IsNotFound(err) {
			return fmt.Errorf("schedule subscription %d: %w", record.ID, err)
		}
		service.tasks.NotifyMonitorChanged()
		return nil
	}
	// No cloud drive submits magnets on the user's behalf anymore: remember the
	// pick so the user can grab it, and look again tomorrow. A hash recorded
	// through a manual magnet stays untouched — the user picked it.
	update := record.Update().SetLastCheckedAt(now).SetNextCheckAt(nextDaily(now, checkTime)).
		AddChecks(1).ClearError()
	if record.Hash == "" {
		update.SetHash(best.Hash)
	}
	if err := update.Exec(ctx); err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("update subscription %d: %w", record.ID, err)
	}
	slog.InfoContext(ctx, "subscribed movie magnet found", "code", record.Code, "hash", best.Hash)
	*hits = append(*hits, hitLine(record.Code, record.Title))
	service.tasks.NotifyMonitorChanged()
	return nil
}

// notifyHits pushes the daily check outcome to the configured channel. Hits
// carry code and title; failures are counted only. Sending runs in the
// background with its own timeout so the scheduler is never delayed, and a
// failed delivery is logged without affecting the check result.
func (service *Service) notifyHits(ctx context.Context, cfg Config, hits []string, failures int) {
	if len(hits) == 0 || !cfg.Notification.Enabled {
		return
	}
	message := fmt.Sprintf("命中 %d 部：\n%s", len(hits), strings.Join(hits, "\n"))
	if failures > 0 {
		message += fmt.Sprintf("\n失败 %d 项", failures)
	}
	go func() {
		sendCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := cfg.Notification.Send(sendCtx, nil, "Miyabi 订阅磁力命中", message); err != nil {
			slog.WarnContext(sendCtx, "send subscription notification failed", "error", err)
		}
	}()
}

const hitLineLimit = 80

// hitLine renders one hit as "CODE Title", trimming long titles.
func hitLine(code, title string) string {
	line := strings.TrimSpace(code)
	if name := strings.TrimSpace(title); name != "" {
		if len([]rune(name)) > hitLineLimit {
			name = string([]rune(name)[:hitLineLimit]) + "…"
		}
		line += " " + name
	}
	return line
}

func (service *Service) checkActor(ctx context.Context, record *ent.Subscription, checkTime string) error {
	now := time.Now()
	movies, err := service.browseActor(ctx, record.TargetID)
	if err != nil {
		return service.deferCheck(ctx, record, now, domain.E(domain.KindUpstream, "查询演员新作失败："+domain.PublicMessage(err), err))
	}
	today := now.Format(dateLayout)
	cursor := decodeCursor(record.Cursor)
	if !cursor.Initialized {
		// No baseline yet: record the page without treating the back
		// catalogue as new releases.
		cursor = snapshotCursor(movies, today)
	} else {
		for _, movie := range cursor.newWorks(movies) {
			service.spawnMovie(ctx, record, movie)
		}
		cursor = cursor.advance(movies, today)
	}
	if err := record.Update().SetCursor(cursor.encode()).SetLastCheckedAt(now).SetNextCheckAt(nextDaily(now, checkTime)).
		AddChecks(1).ClearError().Exec(ctx); err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("update actor subscription %d: %w", record.ID, err)
	}
	service.tasks.NotifyMonitorChanged()
	return nil
}

// deferCheck records a transient failure and retries within the hour rather
// than consuming the daily slot. The row stores the public message only; the
// returned error keeps the cause for logs.
func (service *Service) deferCheck(ctx context.Context, record *ent.Subscription, now time.Time, cause error) error {
	if err := record.Update().SetLastCheckedAt(now).SetNextCheckAt(now.Add(retryInterval)).
		SetError(domain.PublicMessage(cause)).Exec(ctx); err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("defer subscription %d: %w", record.ID, err)
	}
	service.tasks.NotifyMonitorChanged()
	return fmt.Errorf("subscription %s: %w", record.Code, cause)
}
