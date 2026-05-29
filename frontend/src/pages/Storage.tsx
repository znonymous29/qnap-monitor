import { useQuery } from '@tanstack/react-query'
import { api, StatusResp, DiskInfo, VolumeInfo } from '../api/client'

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

export default function Storage() {
  const { data, error, isLoading } = useQuery<StatusResp>({
    queryKey: ['status'],
    queryFn: api.status,
    refetchInterval: 5000,
  })

  if (isLoading) return <div className="text-slate-500">加载中...</div>
  if (error) return <div className="text-red-600">加载失败: {(error as Error).message}</div>
  if (!data) return null

  const disks = data.disks ?? []
  const volumes = data.volumes ?? []

  // Build disk map
  const diskMap = new Map<string, DiskInfo>()
  for (const d of disks) {
    diskMap.set(d.hdNo, d)
  }

  // Pair volumes with disks
  const storageEntries: { volume: VolumeInfo; disk?: DiskInfo }[] = []
  for (const v of volumes) {
    const hdNo = hdListToHdNo(v.hdList)
    storageEntries.push({ volume: v, disk: diskMap.get(hdNo) })
  }

  // Total capacity across all volumes
  const totalCapacity = volumes.reduce((sum, v) => sum + v.capacityBytes, 0)
  const totalUsed = volumes.reduce((sum, v) => sum + v.usedBytes, 0)
  const totalPct = totalCapacity > 0 ? (totalUsed / totalCapacity) * 100 : 0

  return (
    <div className="space-y-6">
      <h2 className="text-lg font-semibold text-slate-800">存储总览</h2>

      {/* Total capacity summary */}
      {totalCapacity > 0 && (
        <div className="rounded-lg border border-slate-200 bg-white p-5 shadow-sm">
          <div className="flex items-baseline justify-between mb-2">
            <span className="text-sm text-slate-600">总容量</span>
            <span className="text-sm text-slate-500">
              {fmtBytes(totalUsed)} / {fmtBytes(totalCapacity)}
            </span>
          </div>
          <div className="w-full bg-slate-100 rounded-full h-3">
            <div
              className={`h-3 rounded-full transition-all ${
                totalPct > 90 ? 'bg-red-500' : totalPct > 75 ? 'bg-amber-500' : 'bg-blue-500'
              }`}
              style={{ width: `${Math.min(totalPct, 100)}%` }}
            />
          </div>
          <div className="text-right mt-1">
            <span className="text-2xl font-bold text-slate-900">{totalPct.toFixed(1)}%</span>
            <span className="text-sm text-slate-500 ml-1">已使用</span>
          </div>
        </div>
      )}

      {/* Volume details */}
      {storageEntries.length > 0 && (
        <section>
          <h3 className="text-sm font-semibold text-slate-700 mb-3">存储卷</h3>
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            {storageEntries.map(({ volume, disk }) => {
              const usedColor = volume.usedPct > 90 ? 'bg-red-500' : volume.usedPct > 75 ? 'bg-amber-500' : 'bg-blue-500'
              return (
                <div key={volume.volNo} className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
                  <div className="flex items-center justify-between mb-1">
                    <span className="font-medium text-slate-800">{volume.label}</span>
                    <span className="text-xs text-slate-500">卷 {volume.volNo} · {volume.filesystem}</span>
                  </div>
                  {disk && (
                    <div className="text-xs text-slate-500 mb-2">
                      {disk.vendor ? `${disk.vendor} ` : ''}{disk.model} ({disk.hdNo})
                    </div>
                  )}
                  <div className="flex items-baseline justify-between mb-1">
                    <span className="text-xl font-bold text-slate-900">{volume.usedPct.toFixed(1)}%</span>
                    <span className="text-xs text-slate-500">
                      {fmtBytes(volume.usedBytes)} / {fmtBytes(volume.capacityBytes)}
                    </span>
                  </div>
                  <div className="w-full bg-slate-100 rounded-full h-2">
                    <div className={`h-2 rounded-full transition-all ${usedColor}`} style={{ width: `${Math.min(volume.usedPct, 100)}%` }} />
                  </div>
                  <div className="flex justify-between mt-1 text-xs text-slate-400">
                    <span>已用 {fmtBytes(volume.usedBytes)}</span>
                    <span>可用 {fmtBytes(volume.freeBytes)}</span>
                  </div>
                </div>
              )
            })}
          </div>
        </section>
      )}

      {/* Disk inventory */}
      {disks.length > 0 && (
        <section>
          <h3 className="text-sm font-semibold text-slate-700 mb-3">硬盘清单</h3>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-200 text-left">
                  <th className="pb-2 pr-4 font-medium text-slate-600">硬盘</th>
                  <th className="pb-2 pr-4 font-medium text-slate-600">型号</th>
                  <th className="pb-2 pr-4 font-medium text-slate-600">容量</th>
                  <th className="pb-2 pr-4 font-medium text-slate-600">温度</th>
                  <th className="pb-2 pr-4 font-medium text-slate-600">健康</th>
                  <th className="pb-2 pr-4 font-medium text-slate-600">通电时长</th>
                  <th className="pb-2 font-medium text-slate-600">类型</th>
                </tr>
              </thead>
              <tbody>
                {disks.map((d) => (
                  <tr key={d.hdNo} className="border-b border-slate-100">
                    <td className="py-2 pr-4">
                      <div className="font-medium text-slate-800">{d.alias}</div>
                      <div className="text-xs text-slate-400">{d.hdNo}</div>
                    </td>
                    <td className="py-2 pr-4 text-slate-600">
                      {d.vendor ? `${d.vendor} ` : ''}{d.model}
                    </td>
                    <td className="py-2 pr-4 text-slate-700">{d.capacity}</td>
                    <td className="py-2 pr-4">
                      <span className={`font-medium ${
                        d.tempC > 55 ? 'text-red-600' : d.tempC > 45 ? 'text-amber-600' : 'text-slate-700'
                      }`}>
                        {d.tempC}°C
                      </span>
                    </td>
                    <td className="py-2 pr-4">
                      <span className={`font-medium ${d.health === 'OK' ? 'text-green-600' : 'text-red-600'}`}>
                        {d.health}
                      </span>
                    </td>
                    <td className="py-2 pr-4 text-slate-600">{fmtPowerOnHours(d.powerOnHours)}</td>
                    <td className="py-2">
                      <span className={`text-xs px-1.5 py-0.5 rounded ${d.isSsd ? 'bg-purple-100 text-purple-700' : 'bg-blue-100 text-blue-700'}`}>
                        {d.isSsd ? 'SSD' : 'HDD'}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {disks.length === 0 && volumes.length === 0 && (
        <div className="text-slate-500 text-sm">暂无存储数据，请先配置 QNAP 连接。</div>
      )}
    </div>
  )
}
