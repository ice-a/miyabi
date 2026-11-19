import { FolderOpenIcon, HardDriveIcon, LoaderCircleIcon, LogOutIcon } from 'lucide-react'
import { useState } from 'react'

import { useClearStorage, useMountStorage, useStorage } from '@/api/storage'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldLabel
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { SettingRow, SettingsSection } from './shared'

export function StorageSection() {
  const storage = useStorage()
  const mount = useMountStorage()
  const clear = useClearStorage()
  const [path, setPath] = useState('')
  const [pathError, setPathError] = useState<string | null>(null)
  const directory = storage.data?.mounted ? storage.data.directory : null

  function submit() {
    const next = path.trim()
    if (next === '') {
      setPathError('请输入目录')
      return
    }
    mount.mutate(
      { path: next },
      {
        onSuccess: () => {
          setPath('')
          setPathError(null)
        }
      }
    )
  }

  let description = '尚未挂载媒体目录'
  if (storage.isPending) {
    description = '正在读取挂载状态…'
  } else if (storage.isError) {
    description = '暂时无法读取挂载状态，请检查后端服务后重试'
  } else if (directory) {
    description = directory.path
  }

  return (
    <SettingsSection icon={<HardDriveIcon className="size-4" />} title="媒体目录">
      <SettingRow title="当前目录" description={description} inline>
        <div className="flex items-center gap-2">
          {storage.isError ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={storage.isFetching}
              onClick={() => void storage.refetch()}
            >
              {storage.isFetching ? <LoaderCircleIcon className="animate-spin" /> : null}
              重试
            </Button>
          ) : null}
          {directory ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={clear.isPending}
              onClick={() => clear.mutate()}
            >
              {clear.isPending ? (
                <LoaderCircleIcon className="animate-spin" />
              ) : (
                <LogOutIcon />
              )}
              取消挂载
            </Button>
          ) : null}
        </div>
      </SettingRow>

      <form
        className="space-y-3"
        onSubmit={event => {
          event.preventDefault()
          submit()
        }}
      >
        <Field data-invalid={Boolean(pathError)}>
          <FieldLabel htmlFor="storage-path">{directory ? '更换目录' : '挂载目录'}</FieldLabel>
          <FieldContent className="flex items-start gap-2">
            <Input
              id="storage-path"
              value={path}
              disabled={mount.isPending}
              aria-invalid={Boolean(pathError)}
              placeholder="/mnt/media"
              onChange={event => {
                setPath(event.target.value)
                setPathError(null)
              }}
            />
            <Button type="submit" size="sm" disabled={mount.isPending}>
              {mount.isPending ? (
                <LoaderCircleIcon className="animate-spin" />
              ) : (
                <FolderOpenIcon />
              )}
              挂载
            </Button>
          </FieldContent>
          <FieldDescription>
            填写本机磁盘或已挂载 NAS 的绝对路径。目录内的影片、NFO 与字幕会用于扫描和刮削。
          </FieldDescription>
          {pathError ? <FieldError>{pathError}</FieldError> : null}
        </Field>
      </form>

      {mount.isError ? <InlineError>挂载失败：{mount.error.message}</InlineError> : null}
      {clear.isError ? <InlineError>取消挂载未完成，请检查后端服务后重试。</InlineError> : null}
    </SettingsSection>
  )
}