import { useEffect, useRef } from 'react'
import { toast } from 'sonner'

import { useTasks } from '@/api/tasks'
import { useTaskConnection } from './task-events'
import { diffTaskNotifications, type NotificationEntry } from './task-notification-diff'
import { notifyBatchTask, notifyScanTask } from './task-toast'

export function TaskNotifications() {
  const tasks = useTasks()
  const connection = useTaskConnection()
  const previous = useRef(new Map<string, NotificationEntry>())
  const dismissed = useRef(new Set<string>())
  const initialized = useRef(false)

  useEffect(() => {
    if (!tasks.data) return

    const announced = new Set(toast.getToasts().map(item => String(item.id)))
    const result = diffTaskNotifications({
      tasks: tasks.data,
      waiting: connection.status !== 'connected',
      isTasksError: tasks.isError,
      previous: previous.current,
      dismissed: dismissed.current,
      announced,
      initialized: initialized.current
    })

    for (const dismissID of result.dismissIDs) {
      toast.dismiss(dismissID)
      dismissed.current.delete(dismissID)
    }

    for (const action of result.actions) {
      const callbacks = {
        onDismiss: () => {
          dismissed.current.add(action.id)
        }
      }
      if (action.type === 'notify_scan') {
        notifyScanTask(action.task, { ...callbacks, waiting: action.waiting })
      } else if (action.type === 'notify_batch') {
        notifyBatchTask(action.task, { ...callbacks, waiting: action.waiting })
      }
    }

    previous.current = result.nextEntries
    initialized.current = true
  }, [tasks.data, tasks.isError, connection.status])

  return null
}