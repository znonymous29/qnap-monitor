import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { api } from '../api/client'

export default function Settings() {
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['config'], queryFn: api.getConfig })

  const [form, setForm] = useState({
    qnapUrl: '',
    qnapUser: '',
    qnapPassword: '',
    collectIntervalSeconds: 10,
    tempThresholdCelsius: 55,
    diskTempThresholdCelsius: 55,
    cpuTempThresholdCelsius: 75,
    retentionDays: 30,
    wecomWebhookUrl: '',
  })
  const [testMsg, setTestMsg] = useState<string | null>(null)
  const [saveMsg, setSaveMsg] = useState<string | null>(null)

  useEffect(() => {
    if (data) {
      setForm({
        qnapUrl: data.qnapUrl,
        qnapUser: data.qnapUser,
        qnapPassword: '',
        collectIntervalSeconds: data.collectIntervalSeconds,
        tempThresholdCelsius: data.tempThresholdCelsius,
        diskTempThresholdCelsius: data.diskTempThresholdCelsius,
        cpuTempThresholdCelsius: data.cpuTempThresholdCelsius,
        retentionDays: data.retentionDays,
        wecomWebhookUrl: data.wecomWebhookUrl,
      })
    }
  }, [data])

  const save = useMutation({
    mutationFn: () => {
      const patch: Record<string, unknown> = {
        qnapUrl: form.qnapUrl,
        qnapUser: form.qnapUser,
        collectIntervalSeconds: Number(form.collectIntervalSeconds),
        tempThresholdCelsius: Number(form.tempThresholdCelsius),
        diskTempThresholdCelsius: Number(form.diskTempThresholdCelsius),
        cpuTempThresholdCelsius: Number(form.cpuTempThresholdCelsius),
        retentionDays: Number(form.retentionDays),
        wecomWebhookUrl: form.wecomWebhookUrl,
      }
      if (form.qnapPassword) patch.qnapPassword = form.qnapPassword
      return api.putConfig(patch)
    },
    onSuccess: () => {
      setSaveMsg('已保存')
      setTimeout(() => setSaveMsg(null), 3000)
      qc.invalidateQueries({ queryKey: ['config'] })
      qc.invalidateQueries({ queryKey: ['status'] })
    },
    onError: (e) => setSaveMsg(`保存失败：${(e as Error).message}`),
  })

  const test = useMutation({
    mutationFn: () =>
      api.testConfig({
        qnapUrl: form.qnapUrl,
        qnapUser: form.qnapUser,
        qnapPassword: form.qnapPassword,
      }),
    onSuccess: (r) => setTestMsg(r.ok ? '✅ 连接成功' : `❌ 连接失败：${r.error}`),
    onError: (e) => setTestMsg(`❌ 请求失败：${(e as Error).message}`),
  })

  if (isLoading) return <div className="text-slate-500">加载中...</div>

  return (
    <form
      className="max-w-2xl space-y-5"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      <Section title="QNAP 连接">
        <Field label="QNAP 地址" hint="如：http://192.168.1.10:8080">
          <input
            className={inputCls}
            value={form.qnapUrl}
            onChange={(e) => setForm({ ...form, qnapUrl: e.target.value })}
            placeholder="http://nas.local:8080"
          />
        </Field>
        <Field label="用户名">
          <input
            className={inputCls}
            value={form.qnapUser}
            onChange={(e) => setForm({ ...form, qnapUser: e.target.value })}
          />
        </Field>
        <Field
          label="密码"
          hint={data?.passwordSet ? '已设置；留空保留原密码' : '尚未设置'}
        >
          <input
            type="password"
            className={inputCls}
            value={form.qnapPassword}
            onChange={(e) => setForm({ ...form, qnapPassword: e.target.value })}
            placeholder={data?.passwordSet ? '••••••••' : ''}
          />
        </Field>
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={() => test.mutate()}
            disabled={test.isPending}
            className={btnSecondary}
          >
            {test.isPending ? '测试中...' : '测试连接'}
          </button>
          {testMsg && <span className="text-sm">{testMsg}</span>}
        </div>
      </Section>

      <Section title="采集与存储">
        <Field label="采集间隔（秒）">
          <input
            type="number"
            min={1}
            className={inputCls}
            value={form.collectIntervalSeconds}
            onChange={(e) =>
              setForm({ ...form, collectIntervalSeconds: Number(e.target.value) })
            }
          />
        </Field>
        <Field label="数据保留天数">
          <input
            type="number"
            min={1}
            className={inputCls}
            value={form.retentionDays}
            onChange={(e) => setForm({ ...form, retentionDays: Number(e.target.value) })}
          />
        </Field>
      </Section>

      <Section title="告警">
        <Field label="系统温度阈值（°C）" hint="CPU/系统温度超过此值触发告警">
          <input
            type="number"
            step="0.5"
            className={inputCls}
            value={form.tempThresholdCelsius}
            onChange={(e) =>
              setForm({ ...form, tempThresholdCelsius: Number(e.target.value) })
            }
          />
        </Field>
        <Field label="硬盘温度阈值（°C）" hint="硬盘温度超过此值触发告警">
          <input
            type="number"
            step="0.5"
            className={inputCls}
            value={form.diskTempThresholdCelsius}
            onChange={(e) =>
              setForm({ ...form, diskTempThresholdCelsius: Number(e.target.value) })
            }
          />
        </Field>
        <Field label="CPU 温度阈值（°C）" hint="CPU 温度超过此值触发告警">
          <input
            type="number"
            step="0.5"
            className={inputCls}
            value={form.cpuTempThresholdCelsius}
            onChange={(e) =>
              setForm({ ...form, cpuTempThresholdCelsius: Number(e.target.value) })
            }
          />
        </Field>
        <Field label="企业微信 Webhook URL" hint="留空则不发送 webhook 通知">
          <input
            className={inputCls}
            value={form.wecomWebhookUrl}
            onChange={(e) => setForm({ ...form, wecomWebhookUrl: e.target.value })}
            placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..."
          />
        </Field>
      </Section>

      <div className="flex items-center gap-3">
        <button type="submit" disabled={save.isPending} className={btnPrimary}>
          {save.isPending ? '保存中...' : '保存'}
        </button>
        {saveMsg && <span className="text-sm">{saveMsg}</span>}
      </div>
    </form>
  )
}

const inputCls =
  'w-full rounded border border-slate-300 px-3 py-2 text-sm focus:border-slate-900 focus:outline-none'
const btnPrimary =
  'rounded bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-700 disabled:opacity-50'
const btnSecondary =
  'rounded border border-slate-300 bg-white px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50'

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <fieldset className="rounded-lg border border-slate-200 bg-white p-5">
      <legend className="px-2 text-sm font-semibold text-slate-700">{title}</legend>
      <div className="space-y-4">{children}</div>
    </fieldset>
  )
}

function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: React.ReactNode
}) {
  return (
    <label className="block">
      <span className="text-sm font-medium text-slate-700">{label}</span>
      <div className="mt-1">{children}</div>
      {hint && <span className="mt-1 block text-xs text-slate-500">{hint}</span>}
    </label>
  )
}
