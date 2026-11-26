import { RefreshCwIcon, StethoscopeIcon } from 'lucide-react'
import { toast } from 'sonner'

import {
  useLibraryHealth,
  type LibraryHealthIssue,
  type LibraryHealthReport
} from '@/api/library'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from 'cn'
import { SettingRow, SettingsSection } from './shared'

const numberFormat = new Intl.NumberFormat('zh-CN')

const issueLabels: Record<string, string> = {
  missing_file: '文件已不在磁盘上',
  missing_file_summary: '索引中的文件在磁盘上找不到',
  unscraped: '刮削未完成',
  unscraped_summary: '刮削未完成的影片共',
  orphan_movie: '影片没有关联的文件',
  orphan_movie_summary: '没有关联文件的影片共'
}

function issueText(issue: LibraryHealthIssue): string {
  const label = issueLabels[issue.kind] ?? issue.kind
  if (issue.kind.endsWith('_summary')) {
    return `${label} ${numberFormat.format(issue.count ?? 0)} 项。`
  }
  const tail = [issue.code, issue.title, issue.path].filter(Boolean).join(' · ')
  return tail ? `${label}：${tail}` : label
}

function IssueList({ report }: { report: LibraryHealthReport }) {
  if (report.issues.length === 0) {
    return (
      <div className="rounded-lg border border-dashed px-4 py-3 text-xs text-muted-foreground">
        未发现问题，索引与磁盘内容一致。
      </div>
    )
  }
  return (
    <ul className="max-h-72 space-y-1.5 overflow-y-auto rounded-lg border px-4 py-3 text-xs leading-5">
      {report.issues.map((issue, index) => (
        <li key={`${issue.kind}-${index}`} className="wrap-break-word text-muted-foreground">
          {issueText(issue)}
        </li>
      ))}
    </ul>
  )
}

export function HealthSection() {
  const health = useLibraryHealth()

  function runHealthCheck() {
    void health.refetch().then(result => {
      if (result.data && result.data.issues.length === 0) {
        toast.success('体检完成，未发现问题')
      }
    })
  }

  return (
    <SettingsSection icon={<StethoscopeIcon className="size-4" />} title="体检报告">
      <SettingRow
        title="媒体库体检"
        description={
          health.isPending ? (
            <Skeleton className="h-4 w-56 max-w-full" />
          ) : health.data ? (
            `共 ${numberFormat.format(health.data.movies)} 部影片、${numberFormat.format(health.data.files)} 个文件；` +
            (health.data.issues.length === 0
              ? '未发现问题'
              : `发现 ${numberFormat.format(health.data.issues.length)} 项待处理`)
          ) : (
            '检查索引中的失效文件、未完成刮削与孤立影片'
          )
        }
      >
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={health.isFetching}
          onClick={runHealthCheck}
        >
          <RefreshCwIcon className={cn('size-4', health.isFetching && 'animate-spin')} />
          {health.data ? '重新体检' : '运行体检'}
        </Button>
      </SettingRow>
      {health.data ? <IssueList report={health.data} /> : null}
      {health.isError ? (
        <InlineError onRetry={() => void health.refetch()} retrying={health.isFetching}>
          无法读取体检报告，请稍后重试。
        </InlineError>
      ) : null}
    </SettingsSection>
  )
}
