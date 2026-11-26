import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'

import { apiGet, apiPost, apiPut } from '@/api/client'
import type { DiscoverMovieDetail } from '@/api/discover'
import { movieStateKeys } from '@/api/movie-states'
import { refreshQueries } from '@/api/query-refresh'
import { storageKey, type StorageStatus } from '@/api/storage'
import { taskKeys, type LibrarySource, type ScanTask, type Task } from '@/api/tasks'

export const LIBRARY_PAGE_SIZE = 20

export type LibraryEntity = { provider?: string; id?: string; name: string }

export type LibraryMovie = {
  id: number
  code: string
  title: string
  javdb_id?: string
  cover?: string
  poster?: string
  fanart?: string
  release_date?: string
  duration: number
  rating: number
  maker?: LibraryEntity
  series?: LibraryEntity
  director?: LibraryEntity
  actors: LibraryEntity[]
  tags: Array<{ id: number; provider: string; source_id: string; name: string }>
  scrape_status: 'pending' | 'done' | 'failed'
  // History-derived cards omit the flag; the library list always carries it.
  favorite?: boolean
  source?: string
}

type LibraryPage = {
  source?: LibrarySource
  movies: LibraryMovie[]
  total: number
  page: number
  has_more: boolean
}

export const libraryKeys = {
  all: ['library'] as const,
  detail: (id: number) => ['library', 'detail', id] as const,
  movies: (page: number) => ['library', 'movies', page] as const,
  health: ['library', 'health'] as const,
  organize: ['library', 'organize'] as const
}

export type LibraryHealthIssue = {
  kind: string
  code?: string
  title?: string
  path?: string
  count?: number
}

export type LibraryHealthReport = {
  generated_at: string
  movies: number
  files: number
  issues: LibraryHealthIssue[]
}

export type LibraryOrganizeIssue = {
  kind: string
  code?: string
  path?: string
  suggested_name?: string
  count?: number
  file_id?: number
}

export type LibraryOrganizeReport = {
  generated_at: string
  issues: LibraryOrganizeIssue[]
  undoable: boolean
}

export type LibraryOrganizeRename = {
  file_id: number
  from: string
  to: string
  error?: string
}

export type LibraryOrganizeApplyReport = {
  renames: LibraryOrganizeRename[]
  undoable: boolean
}

export function useLibraryHealth() {
  return useQuery({
    queryKey: libraryKeys.health,
    queryFn: ({ signal }) => apiGet<LibraryHealthReport>('/api/library/health', undefined, signal),
    staleTime: 0
  })
}

export function useLibraryOrganize(enabled: boolean) {
  return useQuery({
    queryKey: libraryKeys.organize,
    queryFn: ({ signal }) => apiGet<LibraryOrganizeReport>('/api/library/organize', undefined, signal),
    enabled,
    staleTime: 0
  })
}

export function useApplyOrganizeRenames() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (fileIds: number[]) =>
      apiPost<LibraryOrganizeApplyReport>('/api/library/organize/apply', { file_ids: fileIds }),
    onSuccess: () => refreshQueries(queryClient, libraryKeys.all)
  })
}

export function useUndoOrganizeRenames() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<LibraryOrganizeApplyReport>('/api/library/organize/undo'),
    onSuccess: () => refreshQueries(queryClient, libraryKeys.all)
  })
}

export type LibraryMovieDetail = Omit<DiscoverMovieDetail, 'release_status'> & {
  library_id: number
  scrape_status: LibraryMovie['scrape_status']
  favorite: boolean
}

export function useLibraryMovie(id: number) {
  return useQuery({
    queryKey: libraryKeys.detail(id),
    queryFn: ({ signal }) =>
      apiGet<LibraryMovieDetail>(`/api/library/movies/${id}`, undefined, signal)
  })
}

export function useLibrarySimilar(id: number) {
  return useQuery({
    queryKey: [...libraryKeys.detail(id), 'similar'] as const,
    queryFn: ({ signal }) => apiGet<LibraryMovie[]>(`/api/library/movies/${id}/similar`, undefined, signal),
    staleTime: 60_000
  })
}

export function useLibraryMovies(page: number, options?: { favorite?: boolean }) {
  const favorite = options?.favorite ?? false
  return useQuery({
    queryKey: favorite ? [...libraryKeys.movies(page), 'favorite'] : libraryKeys.movies(page),
    queryFn: ({ signal }) =>
      apiGet<LibraryPage>(
        '/api/library/movies',
        favorite ? { page, limit: LIBRARY_PAGE_SIZE, favorite: 'true' } : { page, limit: LIBRARY_PAGE_SIZE },
        signal
      )
  })
}

export function useSetLibraryFavorite(id: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (favorite: boolean) =>
      apiPut<{ favorite: boolean }>(`/api/library/movies/${id}/favorite`, { favorite }),
    onSuccess: () => refreshQueries(queryClient, libraryKeys.all)
  })
}

export function useTopRatedMovies(limit = 12) {
  return useQuery({
    queryKey: ['library', 'top-rated', limit] as const,
    queryFn: ({ signal }) => apiGet<LibraryMovie[]>('/api/library/top-rated', { limit }, signal)
  })
}

export function useStartLibraryScan() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (rebuild: boolean) =>
      apiPost<ScanTask>(rebuild ? '/api/library/rebuild' : '/api/library/scan'),
    onMutate: () => queryClient.getQueryData<Task[]>(taskKeys.all),
    onSuccess: (task, _rebuild, previous) => {
      cacheScanTask(queryClient, task, previous)
      return refreshQueries(queryClient, taskKeys.all)
    }
  })
}

export function useSyncLibrary() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<{ started: number }>('/api/library/sync'),
    onSuccess: () => refreshQueries(queryClient, taskKeys.all, libraryKeys.all)
  })
}

export function useLibraryPending() {
  return useQuery({
    queryKey: ['library', 'pending'] as const,
    queryFn: ({ signal }) => apiGet<{ count: number }>('/api/library/pending', undefined, signal),
    staleTime: 15_000
  })
}

export function useRescrapeLibraryMovie(id: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (code?: string) => apiPost<ScanTask>(`/api/library/movies/${id}/scrape`, { code }),
    onMutate: () => queryClient.getQueryData<Task[]>(taskKeys.all),
    onSuccess: (task, _code, previous) => {
      cacheScanTask(queryClient, task, previous)
      return refreshQueries(queryClient, libraryKeys.all, taskKeys.all, movieStateKeys.all)
    }
  })
}

function cacheScanTask(queryClient: QueryClient, task: ScanTask, previous: Task[] | undefined) {
  const storage = queryClient.getQueryData<StorageStatus>(storageKey)
  if (storage?.mounted && storage.directory.id !== task.source.directory.id) return
  const state = queryClient.getQueryState(taskKeys.all)
  if ((state?.fetchStatus ?? 'idle') !== 'idle' || state?.data !== previous) return
  queryClient.setQueryData<Task[]>(taskKeys.all, tasks => [
    task,
    ...(tasks ?? []).filter(item => item.id !== task.id)
  ])
}
