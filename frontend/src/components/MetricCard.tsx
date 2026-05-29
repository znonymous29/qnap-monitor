interface Props {
  label: string
  value: string
  unit?: string
  hint?: string
  tone?: 'normal' | 'warn' | 'danger'
}

const toneStyles: Record<NonNullable<Props['tone']>, string> = {
  normal: 'border-slate-200',
  warn: 'border-amber-400',
  danger: 'border-red-500 bg-red-50',
}

export default function MetricCard({ label, value, unit, hint, tone = 'normal' }: Props) {
  return (
    <div className={`rounded-lg border-2 bg-white p-5 shadow-sm ${toneStyles[tone]}`}>
      <div className="text-xs text-slate-500 font-medium uppercase tracking-wide">{label}</div>
      <div className="mt-2 flex items-baseline gap-1">
        <span className="text-4xl font-bold text-slate-900">{value}</span>
        {unit && <span className="text-base text-slate-500">{unit}</span>}
      </div>
      {hint && <div className="mt-1 text-xs text-slate-500">{hint}</div>}
    </div>
  )
}
