import { FolderSearchIcon, RefreshCwIcon, Undo2Icon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'

import {
  useApplyOrganizeRenames,
  useLibraryOrganize,
  useUndoOrganizeRenames,
  type LibraryOrganizeIssue,
  type LibraryOrganizeReport
} from '@/api/library'
import { Checkbox } from '@/components/ui/checkbox'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from 'cn'
import { SettingRow, SettingsSection } from './shared'

const numberFormat = new Intl.NumberFormat('zh-CN')

const issueLabels: Record<string, string> = {
  misnamed_file: '文件名缺少番号',
  misnamed_file_summary: '命名不规范的文件共'
}

function issueText(issue: LibraryOrganizeIssue): string {
  const label = issueLabels[issue.kind] ?? issue.kind
  if (issue.kind.endsWith('_summary')) {
    return `${label} ${numberFormat.format(issue.count ?? 0)} 项。`
  }
  const tail = [issue.code, issue.suggested_name, issue.path].filter(Boolean).join(' · ')
  return tail ? `${label}：${tail}` : label
}

function IssueList({
  report,
  selected,
  onToggle
}: {
  report: LibraryOrganizeReport
  selected: Set<number>
  onToggle: (fileId: number) => void
}) {
  const renameable = report.issues.filter(issue => issue.kind === 'misnamed_file' && issue.file_id)
  if (report.issues.length === 0) {
    return (
      <div className="rounded-lg border border-dashed px-4 py-3 text-xs text-muted-foreground">
        未发现命名问题，文件名均已包含番号。
      </div>
    )
  }
  return (
    <ul className="max-h-72 space-y-1.5 overflow-y-auto rounded-lg border px-4 py-3 text-xs leading-5">
      {report.issues.map((issue, index) => (
        <li
          key={`${issue.kind}-${index}`}
          className="wrap-break-word flex items-start gap-2 text-muted-foreground"
        >
          {renameable.includes(issue) ? (
            <Checkbox
              checked={selected.has(issue.file_id as number)}
              onCheckedChange={() => onToggle(issue.file_id as number)}
              aria-label={`选择 ${issue.suggested_name ?? issue.path}`}
              className="mt-0.5"
            />
          ) : null}
          <span>{issueText(issue)}</span>
        </li>
      ))}
    </ul>
  )
}

// Suggestions are computed server-side at apply time; the checkboxes only pick
// candidates, and renames are journalled so the last batch can be undone.
export function OrganizeSection() {
  const organize = useLibraryOrganize(false)
  const apply = useApplyOrganizeRenames()
  const undo = useUndoOrganizeRenames()
  const [selected, setSelected] = useState<Set<number>>(new Set())
  // The report carries the journal state; local overrides only reflect
  // in-session apply/undo results before the next check refetch.
  const [undoOverride, setUndoOverride] = useState<boolean | null>(null)
  const undoable = undoOverride ?? (organize.data?.undoable ?? false)

  function runOrganize() {
    void organize.refetch().then(result => {
      if (result.data && result.data.issues.length === 0) {
        toast.success('整理检查完成，未发现命名问题')
      }
    })
  }

  function toggle(fileId: number) {
    setSelected(previous => {
      const next = new Set(previous)
      if (next.has(fileId)) {
        next.delete(fileId)
      } else {
        next.add(fileId)
      }
      return next
    })
  }

  function applyRenames() {
    const fileIds = [...selected]
    if (fileIds.length === 0) {
      toast.warning('请先勾选要重命名的文件')
      return
    }
    apply.mutate(fileIds, {
      onSuccess: report => {
        const failures = report.renames.filter(rename => rename.error)
        if (report.renames.length === 0) {
          toast.warning('没有可应用的改动，所选文件可能已符合命名')
        } else if (failures.length > 0) {
          toast.error(`${failures.length} 项重命名失败：${failures[0]?.error ?? ''}`)
        } else {
          toast.success(`已重命名 ${report.renames.length} 个文件，可点击撤销恢复`)
        }
        setSelected(new Set())
        setUndoOverride(report.undoable)
      },
      onError: error => {
        toast.error(error instanceof Error ? error.message : '重命名失败，请稍后重试')
      }
    })
  }

  function undoRenames() {
    undo.mutate(undefined, {
      onSuccess: report => {
        toast.success(`已撤销 ${report.renames.length} 个重命名`)
        setUndoOverride(report.undoable)
      },
      onError: error => {
        // The journal entry survives a failed undo; keep the button visible.
        toast.error(error instanceof Error ? error.message : '撤销失败，请稍后重试')
      }
    })
  }

  return (
    <SettingsSection icon={<FolderSearchIcon className="size-4" />} title="文件整理">
      <SettingRow
        title="命名建议"
        description={
          organize.isPending && organize.isFetching ? (
            <Skeleton className="h-4 w-56 max-w-full" />
          ) : organize.data ? (
            organize.data.issues.length === 0
              ? '未发现命名问题'
              : `发现 ${numberFormat.format(organize.data.issues.length)} 项命名建议`
          ) : (
            '按番号检查文件名，勾选后可执行重命名，随时撤销'
          )
        }
      >
        <div className="flex items-center gap-2">
          {undoable ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={undo.isPending}
              onClick={undoRenames}
            >
              <Undo2Icon className={cn('size-4', undo.isPending && 'animate-pulse')} />
              撤销上次重命名
            </Button>
          ) : null}
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={organize.isFetching}
            onClick={runOrganize}
          >
            <RefreshCwIcon className={cn('size-4', organize.isFetching && 'animate-spin')} />
            {organize.data ? '重新检查' : '运行检查'}
          </Button>
        </div>
      </SettingRow>
      {organize.data ? <IssueList report={organize.data} selected={selected} onToggle={toggle} /> : null}
      {organize.data && selected.size > 0 ? (
        <div className="flex justify-end">
          <Button type="button" size="sm" disabled={apply.isPending} onClick={applyRenames}>
            执行重命名（{selected.size}）
          </Button>
        </div>
      ) : null}
      {organize.isError ? (
        <InlineError onRetry={() => void organize.refetch()} retrying={organize.isFetching}>
          无法读取整理建议，请稍后重试。
        </InlineError>
      ) : null}
    </SettingsSection>
  )
}
