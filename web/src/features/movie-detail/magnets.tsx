import { CheckIcon, CopyIcon } from 'lucide-react'
import { useState } from 'react'

import type { DiscoverMagnet, useDiscoverMagnets } from '@/api/discover'
import { InlineError } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { formatSize } from '@/lib/format'
import { MovieSubscriptionAction } from './subscription-action'
import { MovieMagnetsSkeleton } from './skeleton'

const sourceLabels: Record<string, string> = { javdb: 'JavDB', javbus: 'JavBus' }

export function MovieMagnets({
  movieID,
  query
}: {
  movieID: string
  query: ReturnType<typeof useDiscoverMagnets>
}) {
  const [copiedHash, setCopiedHash] = useState('')
  const [copyError, setCopyError] = useState(false)

  async function copy(magnet: DiscoverMagnet) {
    setCopyError(false)
    try {
      await navigator.clipboard.writeText(magnet.uri)
      setCopiedHash(magnet.hash)
    } catch {
      setCopyError(true)
    }
  }

  return (
    <section className="space-y-4">
      <h2 className="text-xl font-semibold tracking-normal">磁力链</h2>
      <p className="text-sm text-muted-foreground">
        复制磁力链后交给你自己的下载工具。下载完成的影片放进已挂载的媒体目录，同步媒体库即可入库。
      </p>
      {query.isPending ? (
        <MovieMagnetsSkeleton />
      ) : query.isError ? (
        <InlineError onRetry={() => void query.refetch()} retrying={query.isFetching}>
          磁力加载失败
        </InlineError>
      ) : query.data.length === 0 ? (
        <MovieSubscriptionAction movieID={movieID} />
      ) : (
        <div className="space-y-4">
          {query.data.map(magnet => (
            <MagnetCard
              key={`${movieID}:${magnet.hash}`}
              magnet={magnet}
              copied={copiedHash === magnet.hash}
              onCopy={() => void copy(magnet)}
            />
          ))}
        </div>
      )}
      {copyError ? <InlineError>复制失败，请重试。</InlineError> : null}
    </section>
  )
}

function MagnetCard({
  magnet,
  copied,
  onCopy
}: {
  magnet: DiscoverMagnet
  copied: boolean
  onCopy: () => void
}) {
  return (
    <Card size="sm">
      <CardContent className="space-y-3">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
          <div className="min-w-0 flex-1 space-y-2">
            <h3 className="text-base leading-6 font-semibold wrap-break-word">{magnet.name}</h3>
            <p className="w-fit max-w-full font-mono text-xs break-all text-muted-foreground select-all">
              {magnet.uri}
            </p>
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm leading-6 text-muted-foreground">
              <span className="text-xs">{formatSize(magnet.size)}</span>
              {magnet.sources?.map(source => (
                <Badge key={source} variant="outline">
                  {sourceLabels[source] ?? source}
                </Badge>
              ))}
              {magnet.tags?.map(tag => (
                <Badge key={tag} variant="outline">
                  {tag}
                </Badge>
              ))}
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-2 self-end sm:self-center">
            <Button type="button" variant="outline" size="sm" onClick={onCopy}>
              {copied ? <CheckIcon /> : <CopyIcon />}
              {copied ? '已复制' : '复制'}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}