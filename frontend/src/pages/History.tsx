import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { api } from '../api/client'
import MetricChart from '../components/MetricChart'

type Tab = 'charts' | 'stats'

const RANGES: { label: string; seconds: number }[] = [
  { label: '1 小时', seconds: 3600 },
  { label: '6 小时', seconds: 6 * 3600 },
  { label: '24 小时', seconds: 24 * 3600 },
  { label: '7 天', seconds: 7 * 24 * 3600 },
  { label: '30 天', seconds: 30 * 24 * 3600 },
]

const DISK_COLORS = ['#2563eb', '#9333ea', '#ea580c', '#16a34a', '#dc2626', '#0891b2']
const VOL_COLORS = ['#2563eb', '#9333ea', '#ea580c', '#16a34a', '#dc2626']

// Convert hd_list "0000:0001" → disk hdNo "0:1"
function hdListToHdNo(hdList: string): string {
  const parts = hdList.split(':')
  if (parts.length === 2) {
    return `0:${parseInt(parts[1], 10)}`
  }
  return hdList
}

export default function History() {
  const [tab, setTab] = useState<Tab>('charts')
  const [rangeSec, setRangeSec] = useState<number>(3600)
  const to = Math.floor(Date.now() / 1000)
  const from = to - rangeSec

  const { data: status } = useQuery({ queryKey: ['status'], queryFn: api.status })
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['metrics', from, to],
    queryFn: () => api.metrics(from, to),
    enabled: tab === 'charts',
  })

  const disks = status?.disks ?? []
  const volumes = status?.volumes ?? []
  const sysThreshold = status?.alert.threshold
  const diskThreshold = status?.alert.diskTempThreshold
  const cpuThreshold = status?.alert.cpuTempThreshold

  // Build disk alias map: hdNo → alias
  const diskAliasMap = new Map<string, string>()
  for (const d of disks) {
    diskAliasMap.set(d.hdNo, d.alias)
  }

  // Map volumes to their disk alias for unified naming
  const volumeDiskNames: Record<number, string> = {}
  for (const v of volumes) {
    const hdNo = hdListToHdNo(v.hdList)
    volumeDiskNames[v.volNo] = diskAliasMap.get(hdNo) ?? v.label
  }

  return (
    <div className="space-y-6">
      {/* Tab switcher */}
      <div className="flex items-center gap-2">
        <button
          onClick={() => setTab('charts')}
          className={`px-3 py-1.5 rounded text-sm ${
            tab === 'charts' ? 'bg-slate-900 text-white' : 'text-slate-600 hover:bg-slate-100'
          }`}
        >
          图表
        </button>
        <button
          onClick={() => setTab('stats')}
          className={`px-3 py-1.5 rounded text-sm ${
            tab === 'stats' ? 'bg-slate-900 text-white' : 'text-slate-600 hover:bg-slate-100'
          }`}
        >
          统计
        </button>
      </div>

      {tab === 'charts' && (
        <>
          <div className="flex flex-wrap items-center gap-3">
            <span className="text-sm text-slate-600">时间范围：</span>
            {RANGES.map((r) => (
              <button
                key={r.seconds}
                onClick={() => setRangeSec(r.seconds)}
                className={`px-3 py-1 rounded text-sm border ${
                  rangeSec === r.seconds
                    ? 'bg-slate-900 text-white border-slate-900'
                    : 'bg-white text-slate-700 border-slate-300 hover:bg-slate-50'
                }`}
              >
                {r.label}
              </button>
            ))}
            <button
              onClick={() => refetch()}
              className="ml-auto px-3 py-1 rounded text-sm border border-slate-300 bg-white hover:bg-slate-50"
            >
              刷新
            </button>
            {data && (
              <span className="text-xs text-slate-500">
                {data.points.length} 个数据点（聚合: {data.bucket}）
              </span>
            )}
          </div>

          {isLoading && <div className="text-slate-500">加载中...</div>}
          {error && <div className="text-red-600">加载失败: {(error as Error).message}</div>}

          {/* System Metrics */}
          {data && data.points.length > 0 && (
            <section>
              <h2 className="text-base font-semibold text-slate-700 mb-3">系统指标</h2>
              <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                <MetricChart title="CPU 使用率" unit="%" data={data.points.map(p => ({ ts: p.ts, value: p.cpuUsage }))} color="#2563eb" domain={[0, 100]} />
                <MetricChart title="内存使用率" unit="%" data={data.points.map(p => ({ ts: p.ts, value: p.memUsage }))} color="#9333ea" domain={[0, 100]} />
                <MetricChart title="系统温度" unit="°C" data={data.points.map(p => ({ ts: p.ts, value: p.sysTempC }))} color="#ea580c" threshold={sysThreshold} />
                {data.points.some(p => p.cpuTempC > 0) && (
                  <MetricChart title="CPU 温度" unit="°C" data={data.points.map(p => ({ ts: p.ts, value: p.cpuTempC }))} color="#dc2626" threshold={cpuThreshold} />
                )}
              </div>
            </section>
          )}

          {/* Disk Temperatures */}
          {disks.length > 0 && (
            <section>
              <h2 className="text-base font-semibold text-slate-700 mb-3">硬盘温度</h2>
              <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                {disks.map((disk, i) => (
                  <DiskTempChart
                    key={disk.hdNo}
                    hdNo={disk.hdNo}
                    alias={disk.alias}
                    from={from}
                    to={to}
                    color={DISK_COLORS[i % DISK_COLORS.length]}
                    threshold={diskThreshold}
                  />
                ))}
              </div>
            </section>
          )}

          {/* Volume Usage */}
          {volumes.length > 0 && (
            <section>
              <h2 className="text-base font-semibold text-slate-700 mb-3">存储卷使用量</h2>
              <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                {volumes.map((vol, i) => (
                  <VolumeUsageChart
                    key={vol.volNo}
                    volNo={vol.volNo}
                    diskAlias={volumeDiskNames[vol.volNo]}
                    from={from}
                    to={to}
                    color={VOL_COLORS[i % VOL_COLORS.length]}
                  />
                ))}
              </div>
            </section>
          )}

          {data && data.points.length === 0 && disks.length === 0 && (
            <div className="text-slate-500">该时间段没有数据。</div>
          )}
        </>
      )}

      {tab === 'stats' && <StatsView />}
    </div>
  )
}

function DiskTempChart({
  hdNo,
  alias,
  from,
  to,
  color,
  threshold,
}: {
  hdNo: string
  alias: string
  from: number
  to: number
  color: string
  threshold?: number
}) {
  const { data, isLoading } = useQuery({
    queryKey: ['diskTemps', hdNo, from, to],
    queryFn: () => api.diskTemps(hdNo, from, to),
  })

  if (isLoading) return <div className="text-sm text-slate-400">加载 {alias}...</div>
  if (!data || data.points.length === 0) return null

  const chartData = data.points.map((p) => ({ ts: p.ts, value: p.tempC }))
  return (
    <MetricChart
      title={alias}
      unit="°C"
      data={chartData}
      color={color}
      threshold={threshold}
    />
  )
}

function VolumeUsageChart({
  volNo,
  diskAlias,
  from,
  to,
  color,
}: {
  volNo: number
  diskAlias: string
  from: number
  to: number
  color: string
}) {
  const { data, isLoading } = useQuery({
    queryKey: ['volumeUsage', volNo, from, to],
    queryFn: () => api.volumeUsage(volNo, from, to),
  })

  if (isLoading) return <div className="text-sm text-slate-400">加载 {diskAlias}...</div>
  if (!data || data.points.length === 0) return null

  const chartData = data.points.map((p) => ({ ts: p.ts, value: +(p.usedBytes / (1024 * 1024 * 1024)).toFixed(2) }))
  return (
    <MetricChart
      title={diskAlias}
      unit="GB"
      data={chartData}
      color={color}
    />
  )
}

const PERIODS: { label: string; value: 'day' | 'week' | 'month' }[] = [
  { label: '日', value: 'day' },
  { label: '周', value: 'week' },
  { label: '月', value: 'month' },
]

function StatsView() {
  const [period, setPeriod] = useState<'day' | 'week' | 'month'>('day')
  const { data, isLoading, error } = useQuery({
    queryKey: ['stats', period],
    queryFn: () => api.stats(period),
  })

  function fmtPeriodStart(ts: number, period: string): string {
    const d = new Date(ts * 1000)
    if (period === 'day') return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    return d.toLocaleDateString()
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <span className="text-sm text-slate-600">统计周期：</span>
        {PERIODS.map((p) => (
          <button
            key={p.value}
            onClick={() => setPeriod(p.value)}
            className={`px-3 py-1 rounded text-sm ${
              period === p.value ? 'bg-slate-900 text-white' : 'text-slate-600 hover:bg-slate-100'
            }`}
          >
            {p.label}
          </button>
        ))}
      </div>

      {isLoading && <div className="text-slate-500">加载中...</div>}
      {error && <div className="text-red-600">加载失败: {(error as Error).message}</div>}

      {data && data.entries.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-200 text-left">
                <th className="pb-2 pr-4 font-medium text-slate-600">时间</th>
                <th className="pb-2 pr-4 font-medium text-slate-600">CPU 均值</th>
                <th className="pb-2 pr-4 font-medium text-slate-600">CPU 峰值</th>
                <th className="pb-2 pr-4 font-medium text-slate-600">内存均值</th>
                <th className="pb-2 pr-4 font-medium text-slate-600">内存峰值</th>
                <th className="pb-2 pr-4 font-medium text-slate-600">温度均值</th>
                <th className="pb-2 font-medium text-slate-600">温度峰值</th>
              </tr>
            </thead>
            <tbody>
              {data.entries.map((e, i) => (
                <tr key={i} className="border-b border-slate-100">
                  <td className="py-2 pr-4 text-slate-700">{fmtPeriodStart(e.periodStart, data.period)}</td>
                  <td className="py-2 pr-4 text-slate-700">{e.cpuAvg.toFixed(1)}%</td>
                  <td className="py-2 pr-4 text-slate-700">{e.cpuMax.toFixed(1)}%</td>
                  <td className="py-2 pr-4 text-slate-700">{e.memAvg.toFixed(1)}%</td>
                  <td className="py-2 pr-4 text-slate-700">{e.memMax.toFixed(1)}%</td>
                  <td className="py-2 pr-4 text-slate-700">{e.tempAvg.toFixed(1)}°C</td>
                  <td className="py-2 text-slate-700">{e.tempMax.toFixed(1)}°C</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {data && data.entries.length === 0 && (
        <div className="text-slate-500 text-sm">该周期暂无统计数据。</div>
      )}
    </div>
  )
}
