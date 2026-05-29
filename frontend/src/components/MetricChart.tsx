import {
  CartesianGrid,
  Line,
  LineChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

interface DataPoint {
  ts: number
  value: number
}

interface Props {
  title: string
  unit: string
  data: DataPoint[]
  color: string
  domain?: [number | 'auto' | 'dataMin' | 'dataMax', number | 'auto' | 'dataMin' | 'dataMax']
  threshold?: number
  valueFormatter?: (v: number) => string
}

function fmtTime(ts: number) {
  const d = new Date(ts * 1000)
  return `${d.getMonth() + 1}/${d.getDate()} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}
const pad = (n: number) => n.toString().padStart(2, '0')

export default function MetricChart({
  title,
  unit,
  data,
  color,
  domain = ['auto', 'auto'],
  threshold,
  valueFormatter,
}: Props) {
  return (
    <div className="bg-white rounded-lg border border-slate-200 p-4 shadow-sm">
      <div className="mb-2 flex items-center justify-between">
        <h3 className="font-semibold text-slate-700">{title}</h3>
        <span className="text-xs text-slate-500">单位: {unit}</span>
      </div>
      <ResponsiveContainer width="100%" height={240}>
        <LineChart data={data} margin={{ top: 5, right: 20, bottom: 5, left: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#e2e8f0" />
          <XAxis
            dataKey="ts"
            tickFormatter={fmtTime}
            tick={{ fontSize: 11 }}
            stroke="#94a3b8"
            minTickGap={50}
          />
          <YAxis domain={domain} tick={{ fontSize: 11 }} stroke="#94a3b8" width={48} />
          <Tooltip
            labelFormatter={(ts) => fmtTime(Number(ts))}
            formatter={(v: number) => [valueFormatter ? valueFormatter(v) : `${v.toFixed(2)} ${unit}`, title]}
          />
          {threshold !== undefined && (
            <ReferenceLine
              y={threshold}
              stroke="#dc2626"
              strokeDasharray="4 4"
              ifOverflow="extendDomain"
              label={{ value: `阈值 ${threshold}`, fill: '#dc2626', fontSize: 11 }}
            />
          )}
          <Line
            type="monotone"
            dataKey="value"
            stroke={color}
            strokeWidth={2}
            dot={false}
            isAnimationActive={false}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
