export interface Metric {
  ts: number
  cpuUsage: number
  memUsage: number
  sysTempC: number
  cpuTempC: number
  fanRpm: number
  volumeTotalBytes: number
  volumeUsedBytes: number
  volumeUsagePct: number
}

export interface DiskInfo {
  hdNo: string
  alias: string
  model: string
  serial: string
  firmware: string
  vendor: string
  capacity: string
  capacityBytes: number
  health: string
  isSsd: boolean
  tempC: number
  powerOnHours: number
  reallocatedSectors: number
}

export interface VolumeInfo {
  volNo: number
  label: string
  capacityBytes: number
  usedBytes: number
  freeBytes: number
  usedPct: number
  filesystem: string
  raidLevel: number
  hdList: string
}

export interface AlertEvent {
  id: number
  ts: number
  type: 'temperature_high' | 'temperature_recovered'
  value: number
  threshold: number
}

export interface DiskHealthAlert {
  hdNo: string
  health: string
}

export interface SystemInfo {
  model: string
  serialNumber: string
  firmware: string
  uptimeSeconds: number
}

export interface StatusResp {
  configured: boolean
  lastError?: string
  metric?: Metric
  systemInfo?: SystemInfo
  disks?: DiskInfo[]
  volumes?: VolumeInfo[]
  alert: {
    inAlert: boolean
    threshold: number
    diskTempThreshold: number
    cpuTempThreshold: number
    event?: AlertEvent
    diskHealthAlerts?: DiskHealthAlert[]
  }
}

export interface ConfigView {
  qnapUrl: string
  qnapUser: string
  passwordSet: boolean
  collectIntervalSeconds: number
  tempThresholdCelsius: number
  diskTempThresholdCelsius: number
  cpuTempThresholdCelsius: number
  retentionDays: number
  wecomWebhookUrl: string
  updatedAt: number
}

export interface AlertRow {
  id: number
  ts: number
  endTs: number | null
  type: string
  hdNo: string
  value: number
  peakValue: number | null
  threshold: number
  webhookSent: boolean
  acknowledged: boolean
}

export interface MetricsResp {
  from: number
  to: number
  bucket: string
  points: Metric[]
}

export interface DiskTempPoint {
  ts: number
  hdNo: string
  tempC: number
}

export interface DiskTempsResp {
  hdNo: string
  from: number
  to: number
  bucket: string
  points: DiskTempPoint[]
}

export interface VolumeUsagePoint {
  ts: number
  volNo: number
  label: string
  capacityBytes: number
  usedBytes: number
  freeBytes: number
  usedPct: number
}

export interface VolumeUsageResp {
  volNo: number
  from: number
  to: number
  bucket: string
  points: VolumeUsagePoint[]
}

export interface StatsEntry {
  periodStart: number
  cpuAvg: number
  cpuMax: number
  memAvg: number
  memMax: number
  tempAvg: number
  tempMax: number
}

export interface StatsResp {
  period: string
  entries: StatsEntry[]
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok && res.status !== 204) {
    const text = await res.text()
    throw new Error(`${res.status} ${text}`)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

export const api = {
  status: () => req<StatusResp>('/api/status/current'),
  metrics: (from: number, to: number, bucket?: string) => {
    const qs = new URLSearchParams({ from: String(from), to: String(to) })
    if (bucket) qs.set('bucket', bucket)
    return req<MetricsResp>(`/api/metrics?${qs.toString()}`)
  },
  diskTemps: (hdNo: string, from: number, to: number, bucket?: string) => {
    const qs = new URLSearchParams({ hd_no: hdNo, from: String(from), to: String(to) })
    if (bucket) qs.set('bucket', bucket)
    return req<DiskTempsResp>(`/api/disks/temps?${qs.toString()}`)
  },
  volumeUsage: (volNo: number, from: number, to: number, bucket?: string) => {
    const qs = new URLSearchParams({ vol_no: String(volNo), from: String(from), to: String(to) })
    if (bucket) qs.set('bucket', bucket)
    return req<VolumeUsageResp>(`/api/volumes/usage?${qs.toString()}`)
  },
  getConfig: () => req<ConfigView>('/api/config'),
  putConfig: (patch: Partial<Record<string, unknown>>) =>
    req<ConfigView>('/api/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    }),
  testConfig: (body: { qnapUrl: string; qnapUser: string; qnapPassword: string }) =>
    req<{ ok: boolean; error?: string }>('/api/config/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }),
  alerts: (limit = 50) => req<AlertRow[]>(`/api/alerts?limit=${limit}`),
  ackAlert: (id: number) => req<void>(`/api/alerts/${id}/ack`, { method: 'POST' }),
  stats: (period: 'day' | 'week' | 'month') => req<StatsResp>(`/api/stats?period=${period}`),
}
