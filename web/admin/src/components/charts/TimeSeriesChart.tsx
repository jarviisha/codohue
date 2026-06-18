import { useMemo } from 'react'
import { Card, CardContent } from '@jarviisha/davinci-react-ui'
import {
  Area,
  CartesianGrid,
  ComposedChart,
  Legend,
  ResponsiveContainer,
  Tooltip as RechartsTooltip,
  XAxis,
  YAxis,
} from 'recharts'

type Series = {
  key: string
  label: string
  color: string
  stack?: string
}

type TimeSeriesPoint = {
  ts: string
} & Record<string, number | string | null | undefined>

type TimeSeriesChartProps = {
  data: TimeSeriesPoint[]
  series: Series[]
  height?: number
  /**
   * When true, every series shares stack id `stack` so areas pile rather than
   * overlay. The default `false` overlays areas at 50% opacity so spikes in
   * one series stay legible against the others.
   */
  stacked?: boolean
  /**
   * Override the x-axis tick formatter. Default: HH:MM in local time.
   */
  tickFormatter?: (raw: string) => string
}

const DEFAULT_TICK_FORMATTER = (raw: string) => {
  const d = new Date(raw)
  if (Number.isNaN(d.getTime())) return raw
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

/**
 * TimeSeriesChart is a thin Recharts wrapper for the Fleet + namespace
 * dashboards. Series colors map to Davinci semantic tokens so theme switches
 * recolor the chart automatically. The component is intentionally minimal —
 * complex chart needs (brush, secondary y-axis) compose by reaching into
 * Recharts directly instead of bloating this wrapper.
 */
export default function TimeSeriesChart({
  data,
  series,
  height = 200,
  stacked = false,
  tickFormatter = DEFAULT_TICK_FORMATTER,
}: TimeSeriesChartProps) {
  const formatted = useMemo(
    () => data.map((p) => ({ ...p, _label: tickFormatter(p.ts) })),
    [data, tickFormatter],
  )

  return (
    <Card>
      <CardContent>
        <div style={{ width: '100%', height }}>
          <ResponsiveContainer width="100%" height="100%">
        <ComposedChart data={formatted} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
          <CartesianGrid stroke="var(--davinci-semantic-color-border-subtle)" strokeDasharray="3 3" />
          <XAxis
            dataKey="_label"
            stroke="var(--davinci-semantic-color-foreground-subtle)"
            fontSize={11}
            tickLine={false}
            axisLine={false}
          />
          <YAxis
            stroke="var(--davinci-semantic-color-foreground-subtle)"
            fontSize={11}
            tickLine={false}
            axisLine={false}
            allowDecimals={false}
            width={32}
          />
          <RechartsTooltip
            contentStyle={{
              background: 'var(--davinci-semantic-color-surface-raised)',
              border: '1px solid var(--davinci-semantic-color-border)',
              borderRadius: 4,
              fontSize: 12,
            }}
            labelStyle={{ color: 'var(--davinci-semantic-color-foreground)' }}
          />
          <Legend
            wrapperStyle={{ fontSize: 12, color: 'var(--davinci-semantic-color-foreground-subtle)' }}
            iconType="circle"
          />
          {series.map((s) => (
            <Area
              key={s.key}
              type="monotone"
              dataKey={s.key}
              name={s.label}
              stroke={s.color}
              fill={s.color}
              fillOpacity={stacked ? 0.7 : 0.25}
              stackId={stacked ? 'stack' : s.stack}
              strokeWidth={1.5}
            />
          ))}
          </ComposedChart>
        </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  )
}
