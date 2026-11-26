import { ClipboardCopyIcon, LinkIcon, LoaderCircleIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'

import {
  type SubscriptionItem,
  useRemoveSubscription,
  useSetSubscriptionMagnet
} from '@/api/subscriptions'
import { describeApiError } from '@/api/client'
import { MovieCard } from '@/components/movie'
import { MovieStateBadge } from '@/components/movie/movie-badges'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { MovieDetailTrigger } from '@/features/movie-detail/detail-trigger'
import { cn } from 'cn'

const dateTimeFormat = new Intl.DateTimeFormat('zh-CN', {
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit'
})

const statusLabels: Record<SubscriptionItem['status'], string> = {
  waiting: '等待磁力',
  stale: '长期无源',
  active: '生效中',
  paused: '已暂停'
}

// Keep the card mounted when switching between browsing and selection.
export function SubscriptionCard({
  item,
  selecting,
  selected,
  disabled,
  onSelect
}: {
  item: SubscriptionItem
  selecting: boolean
  selected: boolean
  disabled: boolean
  onSelect: () => void
}) {
  const remove = useRemoveSubscription()
  const busy = disabled || remove.isPending

  const card = (
    <MovieCard
      movie={item}
      description={
        <div className="space-y-1">
          <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
            <span>{item.release_date ? `发行 ${item.release_date}` : '发行日期未知'}</span>
            <span className="tabular-nums">{describeSchedule(item)}</span>
          </div>
          {item.error ? (
            <p className="line-clamp-2 text-destructive" title={item.error}>
              {item.error}
            </p>
          ) : null}
        </div>
      }
      state={
        <>
          <MovieStateBadge movie={{ id: item.target_id, code: item.code }} hideViewed />
          <Badge variant={statusVariant(item.status)}>{statusLabels[item.status]}</Badge>
        </>
      }
      coverOverlay={
        !selecting ? (
          <div className="absolute top-2 right-2 flex flex-col gap-1.5">
            {item.kind === 'movie' ? <MagnetControls item={item} /> : null}
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant="secondary"
                  size="icon-sm"
                  disabled={busy}
                  onClick={event => {
                    event.preventDefault()
                    event.stopPropagation()
                    remove.mutate(item)
                  }}
                >
                  {remove.isPending ? (
                    <LoaderCircleIcon className="animate-spin" />
                  ) : (
                    <Trash2Icon />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent side="left">取消订阅</TooltipContent>
            </Tooltip>
          </div>
        ) : undefined
      }
    />
  )

  return (
    <div className="relative h-full min-w-0">
      <MovieDetailTrigger
        movie={{ id: item.target_id }}
        className={cn(
          'block h-full rounded-2xl outline-ring',
          selected && 'ring-2 ring-success',
          selecting && disabled && 'cursor-not-allowed opacity-50'
        )}
        role={selecting ? 'checkbox' : undefined}
        disabled={selecting && disabled}
        onClick={event => {
          if (!selecting) return
          event.preventDefault()
          onSelect()
        }}
      >
        {card}
      </MovieDetailTrigger>
      {selecting ? (
        <Checkbox
          checked={selected}
          disabled={disabled}
          onCheckedChange={onSelect}
          className="absolute top-3 right-3 size-5 data-checked:border-success data-checked:bg-success data-checked:text-white"
        />
      ) : null}
    </div>
  )
}

// Owns the manual-magnet dialog so the card itself stays free of dialog state.
function MagnetControls({ item }: { item: SubscriptionItem }) {
  const setMagnet = useSetSubscriptionMagnet()
  const [open, setOpen] = useState(false)
  const [magnetInput, setMagnetInput] = useState(item.magnet ?? '')
  const magnetBusy = setMagnet.isPending

  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="secondary"
            size="icon-sm"
            onClick={event => {
              event.preventDefault()
              event.stopPropagation()
              setMagnetInput(item.magnet ?? '')
              setOpen(true)
            }}
          >
            <LinkIcon />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="left">手动磁力</TooltipContent>
      </Tooltip>
      <Dialog
        open={open}
        onOpenChange={next => {
          if (!magnetBusy) setOpen(next)
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>手动磁力</DialogTitle>
            <DialogDescription>
              {item.code ? `${item.code} · ` : ''}
              粘贴磁力链接或裸哈希后保存；订阅仍保持等待入库，磁力链可随时复制。
            </DialogDescription>
          </DialogHeader>
          {item.magnet ? (
            <Button
              type="button"
              variant="outline"
              className="justify-start"
              disabled={magnetBusy}
              onClick={() => void navigator.clipboard.writeText(item.magnet ?? '')}
            >
              <ClipboardCopyIcon />
              <span className="truncate font-mono text-xs">{item.magnet}</span>
            </Button>
          ) : null}
          <form
            className="space-y-6"
            onSubmit={event => {
              event.preventDefault()
              if (magnetBusy || !magnetInput.trim()) return
              setMagnet.mutate(
                { id: item.id, magnet: magnetInput.trim() },
                { onSuccess: () => setOpen(false) }
              )
            }}
          >
            <Input
              value={magnetInput}
              onChange={event => setMagnetInput(event.target.value)}
              placeholder="magnet:?xt=urn:btih:…"
              maxLength={4096}
              disabled={magnetBusy}
              autoFocus
              spellCheck={false}
            />
            {setMagnet.error ? (
              <p className="text-destructive">{describeApiError(setMagnet.error)}</p>
            ) : null}
            <DialogFooter>
              <Button type="button" variant="outline" disabled={magnetBusy} onClick={() => setOpen(false)}>
                取消
              </Button>
              <Button type="submit" disabled={magnetBusy || !magnetInput.trim()}>
                {magnetBusy ? '保存中…' : '保存'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}

function statusVariant(status: SubscriptionItem['status']) {
  if (status === 'stale' || status === 'paused') return 'secondary' as const
  return 'default' as const
}

function describeSchedule(item: SubscriptionItem) {
  if (item.status === 'stale') return '发行 30 天后仍无磁力'
  // A recorded hash (manual entry or a daily-check hit) is always worth
  // surfacing, whatever the download mode: nothing is submitted anymore.
  if (item.magnet || item.hash) return '已有磁力，等待入库'
  if (item.next_check_at) {
    const next = new Date(item.next_check_at)
    if (next.getTime() <= Date.now()) return '即将检查'
    return `下次检查 ${dateTimeFormat.format(next)}`
  }
  return '等待检查'
}
