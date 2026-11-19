import { useState } from 'react'
import { HardDriveIcon, PencilIcon, PlusIcon } from 'lucide-react'

import { useSaveSources, useSources, type SourceKind, type StorageSource } from '@/api/sources'
import { useStorage } from '@/api/storage'
import { InlineError } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldLabel
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { SettingsSection } from '@/features/settings/shared'

const KIND_LABEL = '本地目录'

function sourcePath(source: StorageSource) {
  return source.config?.path ?? ''
}

function newSourceID() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `source-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

export function SourcesSection() {
  const sources = useSources()
  const save = useSaveSources()
  const storage = useStorage()
  const [draft, setDraft] = useState<StorageSource | null>(null)
  const [draftKey, setDraftKey] = useState(0)
  const [open, setOpen] = useState(false)
  const mountedPath = storage.data?.mounted ? storage.data.directory.path : ''

  const persist = (next: StorageSource[], onSuccess?: () => void) =>
    save.mutate(next, { onSuccess })

  function openDraft(source: StorageSource) {
    setDraft(source)
    setDraftKey(key => key + 1)
    setOpen(true)
  }

  function submit(source: StorageSource) {
    const list = sources.data ?? []
    if (source.id) {
      persist(
        list.map(item => (item.id === source.id ? source : item)),
        () => setOpen(false)
      )
    } else {
      persist([...list, { ...source, id: newSourceID() }], () => setOpen(false))
    }
  }

  function remove(id: string) {
    persist(
      (sources.data ?? []).filter(item => item.id !== id),
      () => setOpen(false)
    )
  }

  return (
    <SettingsSection icon={<HardDriveIcon className="size-4" />} title="存储源">
      <div className="flex justify-end">
        <Button
          onClick={() =>
            openDraft({
              id: '',
              kind: 'local',
              name: '',
              enabled: true,
              config: mountedPath ? { path: mountedPath } : {}
            })
          }
        >
          <PlusIcon data-icon="inline-start" />
          添加源
        </Button>
      </div>

      {sources.isError ? (
        <InlineError
          onRetry={() => void sources.refetch()}
          retrying={sources.isFetching}
          retryLabel="重试"
        >
          无法读取存储源
        </InlineError>
      ) : null}
      {save.isError ? <InlineError>保存失败，请重试</InlineError> : null}

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {(sources.data ?? []).map(source => (
          <SourceCard key={source.id} source={source} onEdit={() => openDraft(source)} />
        ))}
        {(sources.data?.length ?? 0) === 0 && !sources.isPending && !sources.isError ? (
          <Card size="sm" className="sm:col-span-2">
            <CardContent>
              <CardDescription>还没有存储源，点击「添加源」开始。</CardDescription>
            </CardContent>
          </Card>
        ) : null}
      </div>

      {draft ? (
        <SourceDialog
          key={draftKey}
          source={draft}
          open={open}
          saving={save.isPending}
          onOpenChange={setOpen}
          onSubmit={submit}
          onDelete={() => remove(draft.id)}
        />
      ) : null}
    </SettingsSection>
  )
}

function SourceCard({ source, onEdit }: { source: StorageSource; onEdit: () => void }) {
  const path = sourcePath(source)

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <span className="truncate">{source.name}</span>
          <Badge variant="secondary">{KIND_LABEL}</Badge>
        </CardTitle>
        {path ? <CardDescription className="truncate">{path}</CardDescription> : null}
        <CardAction>
          <Button variant="ghost" size="icon-sm" aria-label="编辑存储源" onClick={onEdit}>
            <PencilIcon />
          </Button>
        </CardAction>
      </CardHeader>
    </Card>
  )
}

function SourceDialog({
  source,
  open,
  saving,
  onOpenChange,
  onSubmit,
  onDelete
}: {
  source: StorageSource
  open: boolean
  saving: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (source: StorageSource) => void
  onDelete: () => void
}) {
  const storage = useStorage()
  const mountedPath = storage.data?.mounted ? storage.data.directory.path : ''
  const [name, setName] = useState(source.name)
  const [draftPath, setPath] = useState(sourcePath(source))
  const [enabled, setEnabled] = useState(source.enabled)
  const [nameError, setNameError] = useState<string | null>(null)
  const [pathError, setPathError] = useState<string | null>(null)
  // Fall back to the mounted directory until the user types their own path.
  const path = draftPath === '' ? mountedPath : draftPath

  function submit() {
    let valid = true
    if (name.trim() === '') {
      setNameError('请输入名称')
      valid = false
    }
    if (path.trim() === '') {
      setPathError('请输入目录')
      valid = false
    }
    if (!valid) return
    const kind: SourceKind = 'local'
    onSubmit({
      id: source.id,
      kind,
      name: name.trim(),
      enabled,
      config: { ...source.config, path: path.trim() }
    })
  }

  return (
    <Dialog
      open={open}
      onOpenChange={nextOpen => {
        if (!nextOpen && !saving) onOpenChange(false)
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{source.id ? '编辑存储源' : '添加存储源'}</DialogTitle>
          <DialogDescription>登记一个本机或 NAS 上的媒体目录。</DialogDescription>
        </DialogHeader>

        <form
          className="flex flex-col gap-4"
          onSubmit={event => {
            event.preventDefault()
            submit()
          }}
        >
          <Field data-invalid={Boolean(nameError)}>
            <FieldLabel htmlFor="source-name">名称</FieldLabel>
            <Input
              id="source-name"
              value={name}
              autoFocus
              disabled={saving}
              aria-invalid={Boolean(nameError)}
              placeholder="我的媒体库"
              onChange={event => {
                setName(event.target.value)
                setNameError(null)
              }}
            />
            {nameError ? <FieldError>{nameError}</FieldError> : null}
          </Field>

          <Field data-invalid={Boolean(pathError)}>
            <FieldLabel htmlFor="source-path">目录路径</FieldLabel>
            <Input
              id="source-path"
              value={path}
              disabled={saving}
              aria-invalid={Boolean(pathError)}
              placeholder="/mnt/media"
              onChange={event => {
                setPath(event.target.value)
                setPathError(null)
              }}
            />
            <FieldDescription>
              {storage.isPending
                ? '正在读取已挂载的媒体目录…'
                : mountedPath
                  ? `已挂载的媒体目录：${mountedPath}`
                  : '尚未在「媒体目录」中挂载目录，可先填写任意绝对路径。'}
            </FieldDescription>
            {pathError ? <FieldError>{pathError}</FieldError> : null}
          </Field>

          <Field orientation="horizontal">
            <FieldContent>
              <FieldLabel htmlFor="source-enabled">启用</FieldLabel>
              <FieldDescription>停用后该源不参与媒体库聚合与扫描</FieldDescription>
            </FieldContent>
            <Switch
              id="source-enabled"
              checked={enabled}
              disabled={saving}
              onCheckedChange={checked => setEnabled(checked)}
            />
          </Field>

          <DialogFooter>
            {source.id ? (
              <Button
                type="button"
                variant="destructive"
                className="mr-auto"
                disabled={saving}
                onClick={onDelete}
              >
                删除
              </Button>
            ) : null}
            <Button
              type="button"
              variant="outline"
              disabled={saving}
              onClick={() => onOpenChange(false)}
            >
              取消
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? (
                <>
                  <Spinner />
                  保存中
                </>
              ) : (
                '保存'
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}