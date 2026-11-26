import { createFileRoute } from '@tanstack/react-router'

import { NotificationSection } from '@/features/settings/notification-section'
import { SubscriptionSection } from '@/features/settings/subscription-section'
import { SettingsCard } from '@/features/settings/shared'

export const Route = createFileRoute('/settings/subscription')({
  component: SettingsSubscriptionPage
})

function SettingsSubscriptionPage() {
  return (
    <SettingsCard>
      <SubscriptionSection />
      <NotificationSection />
    </SettingsCard>
  )
}
