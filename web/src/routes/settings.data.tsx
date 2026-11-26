import { createFileRoute } from '@tanstack/react-router'

import { Separator } from '@/components/ui/separator'
import { DataSection } from '@/features/settings/data-section'
import { HealthSection } from '@/features/settings/health-section'
import { MediaSyncSection } from '@/features/settings/media-sync-section'
import { OrganizeSection } from '@/features/settings/organize-section'
import { TasksSection } from '@/features/settings/tasks-section'
import { SettingsCard } from '@/features/settings/shared'

export const Route = createFileRoute('/settings/data')({
  component: SettingsDataPage
})

function SettingsDataPage() {
  return (
    <SettingsCard>
      <MediaSyncSection />
      <Separator />
      <HealthSection />
      <Separator />
      <OrganizeSection />
      <Separator />
      <TasksSection />
      <Separator />
      <DataSection />
    </SettingsCard>
  )
}
