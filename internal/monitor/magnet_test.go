package monitor

import (
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/subscription"
)

func TestSetManualMagnetKeepsFullURI(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	item, err := f.service.AddMovie(ctx, "m1", AddMovieOptions{})
	if err != nil {
		t.Fatal(err)
	}
	uri := "magnet:?xt=urn:btih:fcef4eb87ac04a85c5285f8377b2856e9e40b9f1&dn=MOCK-m1"
	updated, err := f.service.SetManualMagnet(ctx, item.ID, uri)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Hash != "fcef4eb87ac04a85c5285f8377b2856e9e40b9f1" || updated.Magnet != uri {
		t.Fatalf("SetManualMagnet: hash=%q magnet=%q", updated.Hash, updated.Magnet)
	}
	if updated.Status != subscription.StatusWaiting {
		t.Fatalf("a manual magnet must keep the subscription waiting, got %q", updated.Status)
	}
}

func TestSetManualMagnetAcceptsBareHash(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	item, err := f.service.AddMovie(ctx, "m1", AddMovieOptions{})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := f.service.SetManualMagnet(ctx, item.ID, "FCEF4EB87AC04A85C5285F8377B2856E9E40B9F1")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Hash != "fcef4eb87ac04a85c5285f8377b2856e9e40b9f1" {
		t.Fatalf("hash = %q", updated.Hash)
	}
	if updated.Magnet != "magnet:?xt=urn:btih:"+updated.Hash {
		t.Fatalf("a bare hash should be normalized to a magnet URI, got %q", updated.Magnet)
	}
}

func TestSetManualMagnetRejectsInvalidInput(t *testing.T) {
	f, ctx := newFixture(t), t.Context()
	item, err := f.service.AddMovie(ctx, "m1", AddMovieOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SetManualMagnet(ctx, item.ID, "not-a-magnet"); !domain.IsKind(err, domain.KindInvalid) {
		t.Fatalf("expected invalid-kind error, got %v", err)
	}
	if _, err := f.service.SetManualMagnet(ctx, item.ID, "magnet:?xt=urn:btih:zzzz"); !domain.IsKind(err, domain.KindInvalid) {
		t.Fatalf("expected invalid-kind error for a malformed hash, got %v", err)
	}
	if _, err := f.service.SetManualMagnet(ctx, 9999, "magnet:?xt=urn:btih:fcef4eb87ac04a85c5285f8377b2856e9e40b9f1"); !ent.IsNotFound(err) {
		t.Fatalf("expected not-found error, got %v", err)
	}
}
