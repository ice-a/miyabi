import { MutationCache, QueryCache, QueryClient, type Query } from '@tanstack/react-query'
import { toast } from 'sonner'

import { ApiError, describeApiError, isStorageSourceChanged } from './client'
import { libraryKeys } from './library'
import { storageKey } from './storage'

interface MutationMeta extends Record<string, unknown> {
  errorTitle?: string
}

declare module '@tanstack/react-query' {
  interface Register {
    mutationMeta: MutationMeta
  }
}

export function createAppQueryClient() {
  function refreshStorageState(error: Error, failedQuery?: Query<unknown, unknown>) {
    if (!isStorageSourceChanged(error)) return

    // Storage failures must not trigger a refetch of the storage query itself.
    void client.invalidateQueries(
      { queryKey: storageKey, exact: true, predicate: query => query !== failedQuery },
      { cancelRefetch: false }
    )
    if (!failedQuery) {
      void client.invalidateQueries({ queryKey: libraryKeys.all }, { cancelRefetch: false })
    }
  }

  const client = new QueryClient({
    queryCache: new QueryCache({ onError: refreshStorageState }),
    mutationCache: new MutationCache({
      onError: (error, _variables, _context, mutation) => {
        refreshStorageState(error)
        // The auth gate handles application-session expiry; local errors stay inline.
        if (error instanceof ApiError && error.code === 'UNAUTHORIZED') return
        if (error.name === 'AbortError') return
        if (mutation.meta?.errorTitle) {
          toast.error(mutation.meta.errorTitle, { description: describeApiError(error) })
        }
      }
    }),
    defaultOptions: {
      queries: { staleTime: 30_000, retry: false, refetchOnWindowFocus: false }
    }
  })
  return client
}
