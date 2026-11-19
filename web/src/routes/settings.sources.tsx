import { createFileRoute } from '@tanstack/react-router'

import { Separator } from '@/components/ui/separator'
import { StorageSection } from '@/features/settings/storage-section'
import { SourcesSection } from '@/features/sources/sources-section'
import { SettingsCard } from '@/features/settings/shared'

export const Route = createFileRoute('/settings/sources')({
  component: SettingsSourcesPage
})

function SettingsSourcesPage() {
  return (
    <SettingsCard>
      <StorageSection />
      <Separator />
      <SourcesSection />
    </SettingsCard>
  )
}
