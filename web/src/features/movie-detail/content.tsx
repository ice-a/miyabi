import { StarIcon } from 'lucide-react'

import { useRecordMovieView } from '@/api/browse-history'
import { useDiscoverMagnets, useDiscoverMovie, useResolveDiscoverMovie } from '@/api/discover'
import { type LibraryMovie, useLibraryMovie, useLibrarySimilar, useSetLibraryFavorite } from '@/api/library'
import { MovieCard, MovieGridLayout } from '@/components/movie'
import { Button } from '@/components/ui/button'
import { ErrorState, InlineError } from '@/components/error-state'
import type { MovieDetailTarget } from './dialog-context'
import { MovieHero } from './hero'
import { MovieMagnets } from './magnets'
import { MoviePreviews } from './previews'
import { MovieRecommendations } from './recommendations'
import { MovieDetailSkeleton } from './skeleton'
import { MovieDetailTrigger } from './detail-trigger'

export function MovieDetailContent({ movie }: { movie: MovieDetailTarget }) {
  if ('libraryId' in movie) return <LibraryMovieDetail movieId={movie.libraryId} />
  return 'id' in movie ? (
    <IdentifiedMovieDetail movieId={movie.id} />
  ) : (
    <MovieDetailByCode code={movie.code} />
  )
}

function LibraryMovieDetail({ movieId }: { movieId: number }) {
  const detail = useLibraryMovie(movieId)
  if (detail.isPending) return <MovieDetailSkeleton />
  if (!detail.data) {
    return (
      <ErrorState
        message="媒体库详情加载失败"
        onRetry={() => void detail.refetch()}
        retrying={detail.isFetching}
      />
    )
  }
  return (
    <div className="space-y-10">
      {detail.isRefetchError ? (
        <InlineError onRetry={() => void detail.refetch()} retrying={detail.isFetching}>
          刷新失败，请重试。
        </InlineError>
      ) : null}
      <FavoriteToggle libraryId={detail.data.library_id} favorite={detail.data.favorite} />
      <MovieHero movie={detail.data} libraryStatus={detail.data.scrape_status} />
      <MoviePreviews images={detail.data.preview_images} trailer={detail.data.preview_video} />
      <LibrarySimilar movieId={movieId} />
      {detail.data.id ? <LibraryCatalogueExtras movieId={detail.data.id} /> : null}
    </div>
  )
}

// Library-only: catalogue results fetched by JavDB identity have no library row.
function FavoriteToggle({ libraryId, favorite }: { libraryId: number; favorite: boolean }) {  const toggle = useSetLibraryFavorite(libraryId)
  return (
    <div className="flex justify-end">
      <Button
        size="sm"
        variant={favorite ? 'default' : 'outline'}
        disabled={toggle.isPending}
        onClick={() =>
          toggle.mutate(!favorite, {
            onError: () => toggle.reset()
          })
        }
      >
        <StarIcon className={favorite ? 'fill-current' : undefined} />
        {favorite ? '已收藏' : '收藏'}
      </Button>
    </div>
  )
}

// Local similarity: shared actors/tags/series/maker ranked by the backend.
// Stays quiet when there is nothing to recommend.
function LibrarySimilar({ movieId }: { movieId: number }) {
  const similar = useLibrarySimilar(movieId)
  if (similar.isPending) return null
  const movies = similar.data ?? []
  if (movies.length === 0) return null
  return (
    <section className="space-y-4">
      <h2 className="text-xl font-semibold tracking-normal">相似影片</h2>
      <MovieGridLayout>
        {movies.map(movie => (
          <LibrarySimilarCard key={movie.id} movie={movie} />
        ))}
      </MovieGridLayout>
    </section>
  )
}

function LibrarySimilarCard({ movie }: { movie: LibraryMovie }) {
  return (
    <MovieDetailTrigger movie={{ libraryId: movie.id }} className="relative block h-full">
      <MovieCard
        movie={{ code: movie.code, title: movie.title, cover: movie.cover }}
        description={movie.release_date || '\u00a0'}
        state={movie.favorite ? <span className="text-xs text-muted-foreground">已收藏</span> : undefined}
      />
    </MovieDetailTrigger>
  )
}

// Catalogue extras remain available for linked films, but cannot block or
// replace the saved library metadata when JavDB is unavailable.
function LibraryCatalogueExtras({ movieId }: { movieId: string }) {
  const detail = useDiscoverMovie(movieId)
  const magnets = useDiscoverMagnets(movieId)
  useRecordMovieView(movieId)
  return (
    <>
      <MovieMagnets movieID={movieId} query={magnets} />
      {detail.data ? (
        <>
          <MovieRecommendations title="TA（们）还出演过" movies={detail.data.actor_movies} />
          <MovieRecommendations title="你可能也喜欢" movies={detail.data.related_movies} />
        </>
      ) : null}
    </>
  )
}

function MovieDetailByCode({ code }: { code: string }) {
  const identity = useResolveDiscoverMovie(code)
  if (identity.isPending) return <MovieDetailSkeleton />
  if (!identity.data) {
    return (
      <ErrorState
        message={`未能从 JavDB 确认 ${code} 的影片详情`}
        onRetry={() => void identity.refetch()}
        retrying={identity.isFetching}
      />
    )
  }
  return <IdentifiedMovieDetail key={identity.data.id} movieId={identity.data.id} />
}

function IdentifiedMovieDetail({ movieId }: { movieId: string }) {
  const detail = useDiscoverMovie(movieId)
  const magnets = useDiscoverMagnets(movieId)
  useRecordMovieView(movieId)

  if (detail.isPending) return <MovieDetailSkeleton />
  if (!detail.data) {
    return (
      <ErrorState
        message="影片详情加载失败"
        onRetry={() => void detail.refetch()}
        retrying={detail.isFetching}
      />
    )
  }

  return (
    <div className="space-y-10">
      {detail.isRefetchError ? (
        <InlineError onRetry={() => void detail.refetch()} retrying={detail.isFetching}>
          刷新失败，请重试。
        </InlineError>
      ) : null}
      <MovieHero movie={detail.data} />
      <MoviePreviews images={detail.data.preview_images} trailer={detail.data.preview_video} />
      <MovieMagnets movieID={movieId} query={magnets} />
      <MovieRecommendations title="TA（们）还出演过" movies={detail.data.actor_movies} />
      <MovieRecommendations title="你可能也喜欢" movies={detail.data.related_movies} />
    </div>
  )
}
