import { useMemo, useState } from 'react'
import { BellRingIcon, LoaderCircleIcon } from 'lucide-react'
import { toast } from 'sonner'

import { describeApiError } from '@/api/client'
import {
  type NotificationConfig,
  type NotificationType,
  type SubscriptionConfig,
  useSubscriptionSettings,
  useTestNotification,
  useUpdateSubscriptionSettings
} from '@/api/subscription-settings'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { SettingRow, SettingsSection } from './shared'

const typeOptions: { value: NotificationType; label: string }[] = [
  { value: 'bark', label: 'Bark' },
  { value: 'telegram', label: 'Telegram' },
  { value: 'ntfy', label: 'ntfy' },
  { value: 'webhook', label: 'Webhook' }
]

function configToForm(config?: NotificationConfig | null): NotificationConfig {
  return {
    enabled: config?.enabled ?? false,
    type: config?.type ?? 'bark',
    url: config?.url ?? '',
    bot_token: config?.bot_token ?? '',
    chat_id: config?.chat_id ?? ''
  }
}

export function NotificationSection() {
  const settings = useSubscriptionSettings()
  const update = useUpdateSubscriptionSettings()
  const test = useTestNotification()

  const config = settings.data
  const initial = useMemo(() => configToForm(config?.notification), [config])
  const [form, setForm] = useState<NotificationConfig | null>(null)

  const current = form ?? initial

  const isDirty =
    Boolean(config) &&
    form !== null &&
    (current.enabled !== initial.enabled ||
      current.type !== initial.type ||
      current.url !== initial.url ||
      current.bot_token !== initial.bot_token ||
      current.chat_id !== initial.chat_id)

  if (form !== null && !isDirty && !update.isPending) setForm(null)

  const disabled = settings.isLoading || settings.isError || update.isPending

  function updateField<K extends keyof NotificationConfig>(key: K, value: NotificationConfig[K]) {
    setForm(prev => ({ ...(prev ?? initial), [key]: value }))
  }

  function payload(): SubscriptionConfig | null {
    if (!config) return null
    const notification: NotificationConfig = {
      enabled: current.enabled,
      type: current.type,
      url: current.url.trim(),
      bot_token: current.bot_token.trim(),
      chat_id: current.chat_id.trim()
    }
    if (notification.enabled) {
      if (notification.type === 'telegram' && (!notification.bot_token || !notification.chat_id)) {
        toast.error('启用 Telegram 通知时必须填写 Bot Token 和 Chat ID')
        return null
      }
      if (notification.type !== 'telegram' && !notification.url) {
        toast.error('启用通知时必须填写通知地址')
        return null
      }
    }
    return { ...config, notification }
  }

  function handleSave() {
    const next = payload()
    if (!next) return
    update.mutate(next, {
      onSuccess: () => {
        setForm(null)
        toast.success('通知设置已保存')
      },
      onError: error => toast.error(describeApiError(error))
    })
  }

  function handleTest() {
    const next = payload()
    if (!next) return
    test.mutate(next.notification, {
      onSuccess: () => toast.success('测试通知已发送，请查收'),
      onError: error => toast.error(describeApiError(error))
    })
  }

  return (
    <SettingsSection icon={<BellRingIcon className="size-4" />} title="订阅命中通知">
      <SettingRow
        title="推送订阅命中"
        description="每日检查发现符合偏好的磁力后，把命中清单推送到通知渠道"
        inline
      >
        <Switch
          checked={current.enabled}
          disabled={disabled}
          onCheckedChange={checked => updateField('enabled', checked)}
        />
      </SettingRow>

      {current.enabled ? (
        <>
          <SettingRow title="通知类型" description="Bark、Telegram、ntfy 或通用 Webhook">
            <Select
              value={current.type}
              disabled={disabled}
              onValueChange={value => updateField('type', (value ?? 'bark') as NotificationType)}
            >
              <SelectTrigger className="w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {typeOptions.map(option => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingRow>

          {current.type === 'telegram' ? (
            <>
              <SettingRow title="Bot Token" description="通过 @BotFather 创建机器人后获取">
                <Input
                  type="password"
                  value={current.bot_token}
                  placeholder="123456:ABC-DEF..."
                  disabled={disabled}
                  onChange={e => updateField('bot_token', e.target.value)}
                />
              </SettingRow>
              <SettingRow title="Chat ID" description="接收通知的会话 ID，可通过 @userinfobot 查询">
                <Input
                  value={current.chat_id}
                  placeholder="100000000"
                  disabled={disabled}
                  onChange={e => updateField('chat_id', e.target.value)}
                />
              </SettingRow>
            </>
          ) : (
            <SettingRow
              title="通知地址"
              description={
                current.type === 'bark'
                  ? 'Bark 设备地址，如 https://api.day.app/你的Key'
                  : current.type === 'ntfy'
                    ? 'ntfy 主题地址，如 https://ntfy.sh/my-topic'
                    : '接收 POST JSON {title, message} 的接口地址'
              }
            >
              <Input
                value={current.url}
                placeholder={
                  current.type === 'bark'
                    ? 'https://api.day.app/your-key'
                    : current.type === 'ntfy'
                      ? 'https://ntfy.sh/my-topic'
                      : 'https://example.com/hook'
                }
                disabled={disabled}
                onChange={e => updateField('url', e.target.value)}
              />
            </SettingRow>
          )}

          <div className="flex items-center justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={disabled || test.isPending}
              onClick={handleTest}
            >
              发送测试
            </Button>
            <Button type="button" size="sm" disabled={disabled || !isDirty} onClick={handleSave}>
              {update.isPending ? <LoaderCircleIcon className="animate-spin" /> : null}
              保存
            </Button>
          </div>
        </>
      ) : null}

      {settings.isError ? (
        <InlineError>后端服务暂不可用，无法读取通知设置。</InlineError>
      ) : null}
    </SettingsSection>
  )
}
