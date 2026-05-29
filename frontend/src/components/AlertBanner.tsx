import { DiskHealthAlert } from '../api/client'

interface Props {
  inAlert: boolean
  sysAlert: boolean
  cpuAlert: boolean
  threshold: number
  cpuTempThreshold: number
  currentTemp?: number
  cpuTemp?: number
  diskHealthAlerts?: DiskHealthAlert[]
}

export default function AlertBanner({ inAlert, sysAlert, cpuAlert, threshold, cpuTempThreshold, currentTemp, cpuTemp, diskHealthAlerts }: Props) {
  const hasDiskAlert = diskHealthAlerts && diskHealthAlerts.length > 0
  if (!inAlert && !hasDiskAlert) return null

  return (
    <div className="space-y-2 mb-4">
      {sysAlert && (
        <div className="rounded-lg border-2 border-red-500 bg-red-100 px-4 py-3 flex items-center gap-3">
          <span className="text-2xl">🔥</span>
          <div>
            <div className="font-bold text-red-800">系统温度超过阈值</div>
            <div className="text-sm text-red-700">
              当前温度 <span className="font-semibold">{currentTemp?.toFixed(1)}°C</span>，
              阈值 <span className="font-semibold">{threshold}°C</span>
            </div>
          </div>
        </div>
      )}
      {cpuAlert && (
        <div className="rounded-lg border-2 border-red-500 bg-red-100 px-4 py-3 flex items-center gap-3">
          <span className="text-2xl">🔥</span>
          <div>
            <div className="font-bold text-red-800">CPU 温度超过阈值</div>
            <div className="text-sm text-red-700">
              当前温度 <span className="font-semibold">{cpuTemp?.toFixed(1)}°C</span>，
              阈值 <span className="font-semibold">{cpuTempThreshold}°C</span>
            </div>
          </div>
        </div>
      )}
      {hasDiskAlert && diskHealthAlerts!.map((d) => (
        <div key={d.hdNo} className="rounded-lg border-2 border-amber-500 bg-amber-100 px-4 py-3 flex items-center gap-3">
          <span className="text-2xl">⚠️</span>
          <div>
            <div className="font-bold text-amber-800">硬盘健康异常</div>
            <div className="text-sm text-amber-700">
              硬盘 <span className="font-semibold">{d.hdNo}</span> 状态异常：
              <span className="font-semibold">{d.health}</span>
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}
