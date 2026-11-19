import type { BatchTask, ScanTask, Task } from '../../api/tasks'

function isScanTask(task: Task): task is ScanTask {
  return task.type === 'scan'
}

function isBatchTask(task: Task): task is BatchTask {
  return task.type === 'subscription_batch'
}

function isTaskActive(task: Task): boolean {
  return task.status === 'queued' || task.status === 'running'
}

export type NotificationEntry = {
  active: boolean
  retryable?: boolean
  fingerprint: string
}

export type DiffContext = {
  tasks: Task[]
  waiting: boolean
  isTasksError: boolean
  previous: Map<string, NotificationEntry>
  dismissed: Set<string>
  announced: Set<string>
  initialized: boolean
}

export type NotificationAction =
  | { type: 'notify_scan'; id: string; task: ScanTask; waiting: boolean }
  | { type: 'notify_batch'; id: string; task: BatchTask; waiting: boolean }

export type DiffResult = {
  actions: NotificationAction[]
  dismissIDs: string[]
  nextEntries: Map<string, NotificationEntry>
}

export const scanToastID = (id: number) => `scan:${id}`
export const batchToastID = (id: number) => `batch:${id}`

export function scanFingerprint(task: ScanTask): string {
  return `${task.status}|${task.progress}|${task.error ?? ''}|${task.scan.stage}|${task.scan.movies}|${task.scan.metadata_total}|${task.scan.metadata_completed}|${task.scan.metadata_retrying ?? 0}|${task.scan.metadata_failed ?? 0}|${task.retry_at ?? ''}|${task.updated_at}|${!!task.can_retry}|${!!task.paused}`
}

export function batchFingerprint(task: BatchTask): string {
  return `${task.status}|${task.progress}|${task.error ?? ''}|${task.batch.processed}|${task.batch.failed}|${task.updated_at}|${!!task.can_retry}`
}

export function diffTaskNotifications(ctx: DiffContext): DiffResult {
  const actions: NotificationAction[] = []
  const dismissIDs: string[] = []
  const nextEntries = new Map<string, NotificationEntry>()

  const waitingForScan = ctx.waiting || ctx.isTasksError

  const scans = ctx.tasks.filter(isScanTask)
  const batches = ctx.tasks.filter(isBatchTask)

  for (const task of scans) {
    const id = scanToastID(task.id)
    const active = isTaskActive(task)
    const fp = `${scanFingerprint(task)}|${waitingForScan}`
    const entry: NotificationEntry = { active, fingerprint: fp }
    nextEntries.set(id, entry)

    const old = ctx.previous.get(id)
    if (old?.fingerprint !== fp) {
      // A retry may finish between polls; refresh its visible terminal result too.
      if (
        active
          ? !ctx.dismissed.has(id)
          : old?.active || ctx.announced.has(id) || (!old && ctx.initialized)
      ) {
        actions.push({ type: 'notify_scan', id, task, waiting: waitingForScan })
      }
    }
  }

  for (const task of batches) {
    const id = batchToastID(task.id)
    const active = isTaskActive(task)
    const fp = `${batchFingerprint(task)}|${waitingForScan}`
    const entry: NotificationEntry = { active, fingerprint: fp }
    nextEntries.set(id, entry)

    const old = ctx.previous.get(id)
    if (old?.fingerprint !== fp) {
      if (
        active
          ? !ctx.dismissed.has(id)
          : old?.active || ctx.announced.has(id) || (!old && ctx.initialized)
      ) {
        actions.push({ type: 'notify_batch', id, task, waiting: waitingForScan })
      }
    }
  }

  for (const id of ctx.previous.keys()) {
    if (!nextEntries.has(id)) {
      dismissIDs.push(id)
    }
  }

  return { actions, dismissIDs, nextEntries }
}