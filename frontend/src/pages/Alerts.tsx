import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { api, AlertRow } from '../api/client'

const TYPE_LABELS: Record<string, string> = {
  temperature_high: '系统温度',
  cpu_temperature_high: 'CPU 温度',
  disk_temperature_high: '硬盘温度',
  disk_health_warning: '硬盘健康',
}

const PAGE_SIZE = 20

function fmtTs(ts: number) {
  return new Date(ts * 1000).toLocaleString()
}

function fmtDuration(start: number, end: number | null) {
  if (!end) return '进行中'
  const sec = end - start
  if (sec < 60) return `${sec}秒`
  if (sec < 3600) return `${Math.floor(sec / 60)}分钟`
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return m > 0 ? `${h}小时${m}分钟` : `${h}小时`
}

export default function Alerts() {
  const [page, setPage] = useState(1)
  const { data: alerts, isLoading, error } = useQuery({
    queryKey: ['alerts'],
    queryFn: () => api.alerts(500),
  })
  const { data: status } = useQuery({ queryKey: ['status'], queryFn: api.status })
  const aliasMap = useMemo(() => {
    const m = new Map<string, string>()
    for (const d of status?.disks ?? []) m.set(d.hdNo, d.alias)
    return m
  }, [status?.disks])

  const total = alerts?.length ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const pageAlerts = alerts?.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE) ?? []

  if (isLoading) return <div className="text-slate-500">加载中...</div>
  if (error) return <div className="text-red-600">加载失败: {(error as Error).message}</div>

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-800">告警记录</h2>
        <span className="text-sm text-slate-500">{total} 条记录</span>
      </div>

      {!alerts || alerts.length === 0 ? (
        <div className="text-slate-500 text-sm">暂无告警记录。</div>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-200 text-left">
                  <th className="pb-2 pr-4 font-medium text-slate-600">类型</th>
                  <th className="pb-2 pr-4 font-medium text-slate-600">硬盘</th>
                  <th className="pb-2 pr-4 font-medium text-slate-600">阈值</th>
                  <th className="pb-2 pr-4 font-medium text-slate-600">峰值</th>
                  <th className="pb-2 pr-4 font-medium text-slate-600">开始时间</th>
                  <th className="pb-2 pr-4 font-medium text-slate-600">持续时间</th>
                  <th className="pb-2 font-medium text-slate-600">状态</th>
                </tr>
              </thead>
              <tbody>
                {pageAlerts.map((a) => (
                  <AlertRowComponent key={a.id} a={a} alias={aliasMap.get(a.hdNo)} />
                ))}
              </tbody>
            </table>
          </div>

          {totalPages > 1 && (
            <div className="flex items-center justify-center gap-2 pt-2">
              <button
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page === 1}
                className="px-3 py-1 rounded text-sm border border-slate-300 bg-white hover:bg-slate-50 disabled:opacity-40 disabled:cursor-not-allowed"
              >
                上一页
              </button>
              {Array.from({ length: totalPages }, (_, i) => i + 1)
                .filter((p) => p === 1 || p === totalPages || Math.abs(p - page) <= 2)
                .reduce<(number | 'dots')[]>((acc, p, i, arr) => {
                  if (i > 0 && p - (arr[i - 1] as number) > 1) acc.push('dots')
                  acc.push(p)
                  return acc
                }, [])
                .map((item, i) =>
                  item === 'dots' ? (
                    <span key={`d${i}`} className="px-1 text-slate-400">...</span>
                  ) : (
                    <button
                      key={item}
                      onClick={() => setPage(item as number)}
                      className={`px-3 py-1 rounded text-sm border ${
                        page === item
                          ? 'bg-slate-900 text-white border-slate-900'
                          : 'bg-white text-slate-700 border-slate-300 hover:bg-slate-50'
                      }`}
                    >
                      {item}
                    </button>
                  ),
                )}
              <button
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                disabled={page === totalPages}
                className="px-3 py-1 rounded text-sm border border-slate-300 bg-white hover:bg-slate-50 disabled:opacity-40 disabled:cursor-not-allowed"
              >
                下一页
              </button>
            </div>
          )}
        </>
      )}
    </div>
  )
}

function AlertRowComponent({ a, alias }: { a: AlertRow; alias?: string }) {
  const isActive = a.endTs === null
  const typeLabel = TYPE_LABELS[a.type] ?? a.type

  return (
    <tr className={`border-b border-slate-100 ${isActive ? 'bg-red-50' : ''}`}>
      <td className="py-2.5 pr-4">
        <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${
          a.type === 'temperature_high' ? 'bg-red-100 text-red-700' :
          a.type === 'cpu_temperature_high' ? 'bg-red-100 text-red-700' :
          a.type === 'disk_temperature_high' ? 'bg-orange-100 text-orange-700' :
          'bg-amber-100 text-amber-700'
        }`}>
          {typeLabel}
        </span>
      </td>
      <td className="py-2.5 pr-4 text-slate-700">
        {a.hdNo ? (alias || a.hdNo) : '—'}
      </td>
      <td className="py-2.5 pr-4 text-slate-700">
        {a.type === 'disk_health_warning' ? '—' : `${a.threshold}°C`}
      </td>
      <td className="py-2.5 pr-4 font-medium text-slate-900">
        {a.type === 'disk_health_warning' ? '—' : `${(a.peakValue ?? a.value).toFixed(1)}°C`}
      </td>
      <td className="py-2.5 pr-4 text-slate-600">{fmtTs(a.ts)}</td>
      <td className="py-2.5 pr-4 text-slate-600">{fmtDuration(a.ts, a.endTs)}</td>
      <td className="py-2.5">
        {isActive ? (
          <span className="inline-block px-2 py-0.5 rounded text-xs font-medium bg-red-100 text-red-700">
            进行中
          </span>
        ) : (
          <span className="inline-block px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-700">
            已恢复
          </span>
        )}
      </td>
    </tr>
  )
}
