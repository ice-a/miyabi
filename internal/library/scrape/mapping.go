package scrape

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/predicate"
	"github.com/ppxb/miyabi/internal/ent/tag"
	"github.com/ppxb/miyabi/internal/nfo"
)

// DetailNFO converts a catalogue MovieDetail into an NFO Document.
func DetailNFO(detail domain.MovieDetail) nfo.Movie {
	doc := nfo.Movie{
		Zone:         detail.Zone,
		Title:        detail.Title,
		Code:         detail.Code,
		Premiered:    detail.ReleaseDate,
		Runtime:      detail.Duration,
		Rating:       detail.Rating,
		RatingSource: detail.RatingSource, RatingMax: detail.RatingMax,
		FieldSources: detail.FieldSources, Previews: detail.PreviewImages, PreviewVideo: detail.PreviewVideo,
	}
	for i, source := range detail.Sources {
		doc.IDs = append(doc.IDs, nfo.UniqueID{Type: source.Provider, Value: source.ID, Default: i == 0})
	}
	if detail.Director != nil {
		doc.Director = nfo.Entity{Provider: detail.Director.Provider, ID: detail.Director.ID, Name: detail.Director.Name}
	}
	if detail.Maker != nil {
		doc.Studio = nfo.Entity{Provider: detail.Maker.Provider, ID: detail.Maker.ID, Name: detail.Maker.Name}
	}
	if detail.Series != nil {
		doc.Set = nfo.Series{Provider: detail.Series.Provider, ID: detail.Series.ID, Name: detail.Series.Name}
	}
	for _, person := range detail.Actors {
		doc.Actors = append(doc.Actors, nfo.Actor{
			Provider: person.Provider,
			ID:       person.ID,
			Name:     person.Name,
			NameZHT:  person.NameZHT,
			Gender:   person.Gender,
			Thumb:    person.Avatar,
			Aliases:  person.Aliases,
		})
	}
	for _, item := range detail.Tags {
		doc.Tags = append(doc.Tags, nfo.Tag{
			Provider:   item.Provider,
			ID:         item.ID,
			Name:       item.Name,
			NameZHT:    item.NameZHT,
			CategoryID: item.CategoryID,
		})
		doc.Genres = append(doc.Genres, item.Name)
	}
	return doc
}

// MovieNFO reads the complete document saved by scraping or NFO import.
func MovieNFO(record *ent.Movie) (nfo.Movie, error) {
	if record.Metadata == nil {
		return nfo.Movie{}, domain.E(domain.KindInvalid, "影片资料缺失，请重新刮削", nil)
	}
	return *record.Metadata, nil
}

// SaveMovieMetadata updates an ent.Movie record and associates actors and tags from an NFO document.
func SaveMovieMetadata(ctx context.Context, tx *ent.Tx, id int, doc nfo.Movie) error {
	update := tx.Movie.UpdateOneID(id).SetMetadata(&doc).SetTitle(doc.Title).SetScrapeStatus(movie.ScrapeStatusPending).
		ClearActors().ClearTags().ClearJavdbID().ClearReleaseDate().ClearDuration().ClearRating().
		ClearDirectorID().ClearDirectorName().ClearMakerID().ClearMakerName().ClearSeriesID().ClearSeriesName()
	if doc.Code != "" {
		update.SetCode(doc.Code)
	}
	if doc.JavDBID() != "" {
		update.SetJavdbID(doc.JavDBID())
	}
	if doc.Premiered != "" {
		date, err := time.Parse(time.DateOnly, doc.Premiered)
		if err != nil {
			return fmt.Errorf("parse metadata release date: %w", err)
		}
		update.SetReleaseDate(date)
	}
	if doc.Runtime > 0 {
		update.SetDuration(doc.Runtime)
	}
	if doc.Rating > 0 {
		update.SetRating(doc.Rating)
	}
	if doc.Director.ID != "" {
		update.SetDirectorID(doc.Director.ID)
	}
	if doc.Director.Name != "" {
		update.SetDirectorName(doc.Director.Name)
	}
	if doc.Studio.ID != "" {
		update.SetMakerID(doc.Studio.ID)
	}
	if doc.Studio.Name != "" {
		update.SetMakerName(doc.Studio.Name)
	}
	if doc.Set.ID != "" {
		update.SetSeriesID(doc.Set.ID)
	}
	if doc.Set.Name != "" {
		update.SetSeriesName(doc.Set.Name)
	}
	actors := make([]*ent.ActorCreate, 0, len(doc.Actors))
	actorPredicates := make([]predicate.Actor, 0, len(doc.Actors))
	incoming := make(map[string]nfo.Actor, len(doc.Actors))
	for index, person := range doc.Actors {
		if person.Name == "" {
			continue
		}
		if person.Provider == "" || person.ID == "" {
			person.Provider = "nfo"
			person.ID = doc.Code + ":" + strconv.Itoa(index)
		}
		gender := actor.Gender(person.Gender)
		if gender == "" {
			gender = actor.GenderUnknown
		}
		// Aliases are deliberately not set on the create builder: the upsert
		// would overwrite the merged aliases of an existing row below.
		actors = append(actors, tx.Actor.Create().SetProvider(person.Provider).SetSourceID(person.ID).SetName(person.Name).
			SetNameZht(person.NameZHT).SetGender(gender).SetAvatar(person.Thumb))
		actorPredicates = append(actorPredicates, actor.And(actor.ProviderEQ(person.Provider), actor.SourceIDEQ(person.ID)))
		incoming[actorKey(person.Provider, person.ID)] = person
	}
	if len(actors) > 0 {
		// Capture the stored names before the upsert rewrites them so a renamed
		// actor keeps its earlier spellings as aliases.
		existing, err := tx.Actor.Query().Where(actor.Or(actorPredicates...)).All(ctx)
		if err != nil {
			return err
		}
		conflicts := make(map[string]bool, len(existing))
		for _, record := range existing {
			conflicts[actorKey(record.Provider, record.SourceID)] = true
			person, ok := incoming[actorKey(record.Provider, record.SourceID)]
			if !ok || person.Name == "" {
				continue
			}
			merged := mergeAliases(person.Name, record.Aliases, person.Aliases)
			if person.Name != record.Name {
				merged = mergeAliases(person.Name, merged, []string{record.Name})
			}
			if slices.Equal(merged, record.Aliases) {
				continue
			}
			if err := tx.Actor.Update().Where(actor.IDEQ(record.ID)).SetAliases(merged).Exec(ctx); err != nil {
				return err
			}
		}
		if err := tx.Actor.CreateBulk(actors...).OnConflictColumns(actor.FieldProvider, actor.FieldSourceID).UpdateNewValues().Exec(ctx); err != nil {
			return err
		}
		for rowKey, person := range incoming {
			if conflicts[rowKey] || len(person.Aliases) == 0 {
				continue
			}
			provider, id, _ := strings.Cut(rowKey, "\x00")
			if err := tx.Actor.Update().Where(actor.ProviderEQ(provider), actor.SourceIDEQ(id)).
				SetAliases(mergeAliases(person.Name, person.Aliases)).Exec(ctx); err != nil {
				return err
			}
		}
		ids, err := tx.Actor.Query().Where(actor.Or(actorPredicates...)).IDs(ctx)
		if err != nil {
			return err
		}
		update.AddActorIDs(ids...)
	}
	tags := make([]*ent.TagCreate, 0, len(doc.Tags))
	tagPredicates := make([]predicate.Tag, 0, len(doc.Tags))
	for _, item := range doc.Tags {
		if item.Name == "" {
			continue
		}
		if item.Provider == "" || item.ID == "" {
			item.Provider = "nfo"
			item.ID = item.Name
		}
		tags = append(tags, tx.Tag.Create().SetProvider(item.Provider).SetSourceID(item.ID).SetName(item.Name).SetNameZht(item.NameZHT).SetCategoryID(item.CategoryID))
		tagPredicates = append(tagPredicates, tag.And(tag.ProviderEQ(item.Provider), tag.SourceIDEQ(item.ID)))
	}
	if len(tags) > 0 {
		if err := tx.Tag.CreateBulk(tags...).OnConflictColumns(tag.FieldProvider, tag.FieldSourceID).UpdateNewValues().Exec(ctx); err != nil {
			return err
		}
		ids, err := tx.Tag.Query().Where(tag.Or(tagPredicates...)).IDs(ctx)
		if err != nil {
			return err
		}
		update.AddTagIDs(ids...)
	}
	return update.Exec(ctx)
}

func actorKey(provider, id string) string {
	return provider + "\x00" + id
}

// mergeAliases unions alternate spellings while dropping the current display
// name and empty entries; nil means the actor has no aliases.
func mergeAliases(name string, lists ...[]string) []string {
	seen := map[string]bool{name: true, "": true}
	merged := []string{}
	for _, list := range lists {
		for _, item := range list {
			if seen[item] {
				continue
			}
			seen[item] = true
			merged = append(merged, item)
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}
