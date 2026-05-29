import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { api, StatusResp, DiskInfo, VolumeInfo } from '../api/client'
import MetricCard from '../components/MetricCard'
import AlertBanner from '../components/AlertBanner'

function fmtBytes(n: number): string {
  if (!n) return '0'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(1)} ${units[i]}`
}

function fmtUptime(seconds: number): string {
  if (!seconds) return '—'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}天${h}小时`
  if (h > 0) return `${h}小时${m}分钟`
  return `${m}分钟`
}

function fmtPowerOnHours(hours: number): string {
  if (!hours) return '—'
  const y = Math.floor(hours / 8760)
  const d = Math.floor((hours % 8760) / 24)
  if (y > 0) return `${y}年${d}天`
  if (d > 0) return `${d}天`
  return `${hours}小时`
}

// Convert hd_list "0000:0001" → disk hdNo "0:1"
function hdListToHdNo(hdList: string): string {
  const parts = hdList.split(':')
  if (parts.length === 2) {
    return `0:${parseInt(parts[1], 10)}`
  }
  return hdList
}

export default function Dashboard() {
  const { data, error, isLoading } = useQuery<StatusResp>({
    queryKey: ['status'],
    queryFn: api.status,
    refetchInterval: 5000,
  })

  const [toast, setToast] = useState<string | null>(null)
  useEffect(() => {
    if (data?.alert.event) {
      const e = data.alert.event
      const typeLabels: Record<string, string> = {
        temperature_high: '系统温度',
        cpu_temperature_high: 'CPU 温度',
        disk_temperature_high: '硬盘温度',
      }
      const label = typeLabels[e.type] ?? '温度'
      const msg = `${label}超过阈值！当前 ${e.value.toFixed(1)}°C（阈值 ${e.threshold}°C）`
      setToast(msg)
      const t = setTimeout(() => setToast(null), 6000)
      return () => clearTimeout(t)
    }
  }, [data?.alert.event?.id])

  if (isLoading) return <div className="text-slate-500">加载中...</div>
  if (error) return <div className="text-red-600">加载失败: {(error as Error).message}</div>
  if (!data) return null

  if (!data.configured) {
    return (
      <div className="rounded-lg border border-amber-300 bg-amber-50 p-6">
        <h2 className="text-lg font-bold text-amber-800">还未配置 QNAP 连接</h2>
        <p className="mt-2 text-amber-700">
          请先到 <a href="/settings" className="underline font-medium">设置</a>{' '}
          页面填写 QNAP 的 URL、用户名和密码。
        </p>
      </div>
    )
  }

  const m = data.metric
  const tempTone =
    m && m.sysTempC > data.alert.threshold
      ? 'danger'
      : m && m.sysTempC > data.alert.threshold - 5
        ? 'warn'
        : 'normal'

  // Build disk map for quick lookup
  const diskMap = new Map<string, DiskInfo>()
  for (const d of data.disks ?? []) {
    diskMap.set(d.hdNo, d)
  }

  // Pair volumes with their disks
  const storageEntries: { volume: VolumeInfo; disk?: DiskInfo }[] = []
  for (const v of data.volumes ?? []) {
    const hdNo = hdListToHdNo(v.hdList)
    storageEntries.push({ volume: v, disk: diskMap.get(hdNo) })
  }
  // Disks not associated with any volume
  const usedDiskHdNos = new Set(storageEntries.map((e) => e.disk?.hdNo).filter(Boolean))
  const orphanDisks = (data.disks ?? []).filter((d) => !usedDiskHdNos.has(d.hdNo))

  return (
    <div className="space-y-6">
      <AlertBanner
        inAlert={data.alert.inAlert}
        sysAlert={data.alert.sysAlert}
        cpuAlert={data.alert.cpuAlert}
        threshold={data.alert.threshold}
        cpuTempThreshold={data.alert.cpuTempThreshold}
        currentTemp={m?.sysTempC}
        cpuTemp={m?.cpuTempC}
        diskHealthAlerts={data.alert.diskHealthAlerts}
      />
      {data.lastError && (
        <div className="rounded border border-amber-300 bg-amber-50 px-4 py-2 text-sm text-amber-800">
          上次采集失败：{data.lastError}
        </div>
      )}
      {!m && (
        <div className="text-slate-500">
          已配置，正在等待第一次采集...
        </div>
      )}
      {m && (
        <>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            <MetricCard label="CPU 使用率" value={m.cpuUsage.toFixed(1)} unit="%" />
            <MetricCard label="内存使用率" value={m.memUsage.toFixed(1)} unit="%" />
            <MetricCard
              label="系统温度"
              value={m.sysTempC.toFixed(1)}
              unit="°C"
              hint={`阈值 ${data.alert.threshold}°C`}
              tone={tempTone}
            />
            {m.cpuTempC > 0 && (
              <MetricCard
                label="CPU 温度"
                value={m.cpuTempC.toFixed(1)}
                unit="°C"
                hint={`阈值 ${data.alert.cpuTempThreshold}°C`}
                tone={
                  m.cpuTempC > data.alert.cpuTempThreshold
                    ? 'danger'
                    : m.cpuTempC > data.alert.cpuTempThreshold - 5
                      ? 'warn'
                      : 'normal'
                }
              />
            )}
            {m.fanRpm > 0 && (
              <MetricCard label="风扇转速" value={String(m.fanRpm)} unit="RPM" />
            )}
          </div>
          <div className="text-xs text-slate-500">
            最后采集: {new Date(m.ts * 1000).toLocaleString()}
          </div>
        </>
      )}

      {/* Unified storage: volume + disk paired together */}
      {storageEntries.length > 0 && (
        <section>
          <h2 className="text-lg font-semibold text-slate-800 mb-3">存储</h2>
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            {storageEntries.map(({ volume, disk }) => (
              <StorageCard key={volume.volNo} volume={volume} disk={disk} threshold={data.alert.threshold} diskTempThreshold={data.alert.diskTempThreshold} />
            ))}
          </div>
        </section>
      )}

      {/* Disks without a volume (e.g. unused SSD) */}
      {orphanDisks.length > 0 && (
        <section>
          <h2 className="text-lg font-semibold text-slate-800 mb-3">未分配硬盘</h2>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {orphanDisks.map((d) => (
              <DiskOnlyCard key={d.hdNo} d={d} threshold={data.alert.threshold} diskTempThreshold={data.alert.diskTempThreshold} />
            ))}
          </div>
        </section>
      )}

      {toast && (
        <div className="fixed bottom-6 right-6 max-w-sm rounded-lg bg-slate-900 text-white px-4 py-3 shadow-lg">
          {toast}
        </div>
      )}
    </div>
  )
}

function StorageCard({ volume, disk, threshold, diskTempThreshold }: { volume: VolumeInfo; disk?: DiskInfo; threshold: number; diskTempThreshold: number }) {
  const usedColor = volume.usedPct > 90 ? 'bg-red-500' : volume.usedPct > 75 ? 'bg-amber-500' : 'bg-blue-500'
  const tempTone = disk
    ? disk.tempC > diskTempThreshold ? 'text-red-600' : disk.tempC > diskTempThreshold - 5 ? 'text-amber-600' : 'text-slate-700'
    : ''
  const diskOverThreshold = disk && disk.tempC > diskTempThreshold

  return (
    <div className={`rounded-lg border bg-white p-5 shadow-sm ${diskOverThreshold ? 'border-red-500' : 'border-slate-200'}`}>
      {/* Header: volume label + disk type badge */}
      <div className="flex items-center justify-between mb-1">
        <div className="font-semibold text-slate-800">{volume.label}</div>
        <div className="flex items-center gap-2">
          {disk && (
            <span className={`text-xs px-1.5 py-0.5 rounded ${disk.isSsd ? 'bg-purple-100 text-purple-700' : 'bg-blue-100 text-blue-700'}`}>
              {disk.isSsd ? 'SSD' : 'HDD'}
            </span>
          )}
          <span className="text-xs text-slate-500">卷 {volume.volNo}</span>
        </div>
      </div>

      {/* Disk info row */}
      {disk && (
        <div className="text-xs text-slate-500 mb-3">
          {disk.vendor ? `${disk.vendor} ` : ''}{disk.model} · {disk.capacity} · SN: {disk.serial}
        </div>
      )}

      {/* Usage bar */}
      <div className="mb-3">
        <div className="flex items-baseline justify-between mb-1">
          <span className="text-2xl font-bold text-slate-900">{volume.usedPct.toFixed(1)}%</span>
          <span className="text-sm text-slate-500">
            {fmtBytes(volume.usedBytes)} / {fmtBytes(volume.capacityBytes)}
          </span>
        </div>
        <div className="w-full bg-slate-100 rounded-full h-2.5">
          <div
            className={`h-2.5 rounded-full transition-all ${usedColor}`}
            style={{ width: `${Math.min(volume.usedPct, 100)}%` }}
          />
        </div>
        <div className="flex justify-between mt-1 text-xs text-slate-400">
          <span>已用 {fmtBytes(volume.usedBytes)}</span>
          <span>可用 {fmtBytes(volume.freeBytes)}</span>
        </div>
      </div>

      {/* Disk health, temperature & SMART */}
      {disk && (
        <div className="flex items-center justify-between pt-3 border-t border-slate-100">
          <div className="flex items-center gap-4">
            <div>
              <span className="text-xs text-slate-500">温度</span>
              <span className={`ml-1 text-base font-bold ${tempTone}`}>{disk.tempC}°C</span>
            </div>
            <div>
              <span className="text-xs text-slate-500">健康</span>
              <span className={`ml-1 text-sm font-medium ${disk.health === 'OK' ? 'text-green-600' : 'text-red-600'}`}>
                {disk.health}
              </span>
            </div>
            {disk.powerOnHours > 0 && (
              <div>
                <span className="text-xs text-slate-500">通电</span>
                <span className="ml-1 text-sm text-slate-700">{fmtPowerOnHours(disk.powerOnHours)}</span>
              </div>
            )}
          </div>
          <div className="text-xs text-slate-400">
            {disk.hdNo} · {volume.filesystem}
          </div>
        </div>
      )}
    </div>
  )
}

function DiskOnlyCard({ d, threshold, diskTempThreshold }: { d: DiskInfo; threshold: number; diskTempThreshold: number }) {
  const tempTone = d.tempC > diskTempThreshold ? 'text-red-600' : d.tempC > diskTempThreshold - 5 ? 'text-amber-600' : 'text-slate-900'
  const diskOverThreshold = d.tempC > diskTempThreshold
  return (
    <div className={`rounded-lg border bg-white p-4 shadow-sm ${diskOverThreshold ? 'border-red-500' : 'border-slate-200'}`}>
      <div className="flex items-center justify-between mb-1">
        <div className="font-medium text-slate-800 text-sm">{d.alias}</div>
        <span className={`text-xs px-1.5 py-0.5 rounded ${d.isSsd ? 'bg-purple-100 text-purple-700' : 'bg-blue-100 text-blue-700'}`}>
          {d.isSsd ? 'SSD' : 'HDD'}
        </span>
      </div>
      <div className="text-xs text-slate-500 mb-2">{d.vendor ? `${d.vendor} ` : ''}{d.model} · {d.capacity}</div>
      <div className="flex items-center justify-between">
        <div>
          <span className="text-xs text-slate-500">温度</span>
          <span className={`ml-1 text-lg font-bold ${tempTone}`}>{d.tempC}°C</span>
        </div>
        <div className="text-right">
          <span className="text-xs text-slate-500">健康</span>
          <span className={`ml-1 text-sm font-medium ${d.health === 'OK' ? 'text-green-600' : 'text-red-600'}`}>
            {d.health}
          </span>
        </div>
      </div>
      {d.powerOnHours > 0 && (
        <div className="mt-1 text-xs text-slate-500">
          通电: {fmtPowerOnHours(d.powerOnHours)}
          {d.reallocatedSectors > 0 && (
            <span className="ml-2 text-amber-600">重映射扇区: {d.reallocatedSectors}</span>
          )}
        </div>
      )}
      <div className="mt-1 text-xs text-slate-400">
        {d.hdNo} · SN: {d.serial}
      </div>
    </div>
  )
}
