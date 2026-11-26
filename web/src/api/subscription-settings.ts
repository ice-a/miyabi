import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiGet, apiPost, apiPut } from '@/api/client'

export type PreferenceLevel = 'preferred' | 'required' | 'any'
export type UncensoredFilter = 'preferred' | 'required' | 'exclude' | 'any'

export type MagnetPreferences = {
  subtitle: PreferenceLevel
  hd: PreferenceLevel
  uncensored: UncensoredFilter
}

export type NotificationType = 'bark' | 'telegram' | 'ntfy' | 'webhook'

export type NotificationConfig = {
  enabled: boolean
  type: NotificationType
  url: string
  bot_token: string
  chat_id: string
}

export type SubscriptionConfig = {
  movie_auto_download: boolean
  actor_auto_download: boolean
  check_time: string
  preferences: MagnetPreferences
  download: { auto_switch: boolean }
  notification: NotificationConfig
}

export const subscriptionSettingsKey = ['settings', 'subscription'] as const

export function useSubscriptionSettings() {
  return useQuery({
    queryKey: subscriptionSettingsKey,
    queryFn: ({ signal }) =>
      apiGet<SubscriptionConfig>('/api/settings/subscription', undefined, signal),
    staleTime: 15_000
  })
}

export function useUpdateSubscriptionSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (config: SubscriptionConfig) =>
      apiPut<SubscriptionConfig>('/api/settings/subscription', config),
    onSuccess: next => queryClient.setQueryData(subscriptionSettingsKey, next),
    onError: () => queryClient.invalidateQueries({ queryKey: subscriptionSettingsKey })
  })
}

export function useTestNotification() {
  return useMutation({
    mutationFn: (config: NotificationConfig) => apiPost('/api/settings/notification/test', config)
  })
}
