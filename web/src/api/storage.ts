import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiDelete, apiGet, apiPut } from '@/api/client'
import { resetMovieStates } from '@/api/movie-states'
import { taskKeys } from '@/api/tasks'

export type StorageDirectory = {
  id: string
  name: string
  path: string
}

export type StorageStatus = {
  mounted: boolean
  directory: StorageDirectory
}

export const storageKey = ['settings', 'storage'] as const

export function useStorage() {
  return useQuery({
    queryKey: storageKey,
    queryFn: ({ signal }) => apiGet<StorageStatus>('/api/settings/storage', undefined, signal)
  })
}

export function useMountStorage() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: { path: string; name?: string }) =>
      apiPut<StorageStatus>('/api/settings/storage', input),
    meta: { errorTitle: '挂载失败' },
    onSuccess: status => {
      queryClient.setQueryData(storageKey, status)
      void queryClient.invalidateQueries({ queryKey: ['library'] })
      void queryClient.invalidateQueries({ queryKey: taskKeys.all })
      resetMovieStates(queryClient)
    }
  })
}

export function useClearStorage() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiDelete<StorageStatus>('/api/settings/storage'),
    meta: { errorTitle: '取消挂载失败' },
    onSuccess: status => {
      queryClient.setQueryData(storageKey, status)
      void queryClient.invalidateQueries({ queryKey: ['library'] })
      resetMovieStates(queryClient)
    }
  })
}