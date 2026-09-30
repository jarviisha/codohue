import { useMemo, useState } from 'react'
import {
  Card,
  Section,
  Stack,
  Table,
  proportional,
  TableHeader,
  TableHeaderCell,
  TableRow,
  TableBody,
  TableCell,
  Text,
} from '@astryxdesign/core'
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
import { breakGaps } from '@/components/charts/timeSeries'

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
   * Override the x-axis tick formatter. Default: HH:MM in local time, with
   * the date prepended once the data spans more than a day.
   */
  tickFormatter?: (raw: string) => string
  /**
   * `stepAfter` for series that only record changes (a missing point means
   * "unchanged"), so the line holds flat instead of interpolating.
   */
  curve?: 'monotone' | 'stepAfter'
  /**
   * Break the line when consecutive samples are further apart than this, so
   * an outage in the data source does not render as a steady value.
   */
  maxGapMs?: number
}

const DAY_MS = 24 * 60 * 60 * 1000
const TIME_ZONE = Intl.DateTimeFormat().resolvedOptions().timeZone

const defaultTickFormatter = (withDate: boolean) => (raw: string) => {
  const d = new Date(raw)
  if (Number.isNaN(d.getTime())) return raw
  return withDate
    ? d.toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
    : d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

/**
 * TimeSeriesChart is a thin Recharts wrapper for the Fleet + namespace
 * dashboards. Series colors map to Astryx semantic tokens so theme switches
 * recolor the chart automatically. The component is intentionally minimal —
 * complex chart needs (brush, secondary y-axis) compose by reaching into
 * Recharts directly instead of bloating this wrapper.
 */
export default function TimeSeriesChart({
  data,
  series,
  height = 200,
  stacked = false,
  tickFormatter,
  curve = 'monotone',
  maxGapMs,
}: TimeSeriesChartProps) {
  // The data table can be thousands of rows; keep it out of the DOM until asked for.
  const [tableOpen, setTableOpen] = useState(false)
  // A numeric time axis keeps gaps between samples proportional instead of
  // spacing every point evenly.
  const points = useMemo(() => {
    const withBreaks = maxGapMs
      ? breakGaps(
          data,
          maxGapMs,
          series.map((s) => s.key),
        )
      : data
    return withBreaks.map((p) => ({ ...p, _t: Date.parse(p.ts as string) }))
  }, [data, series, maxGapMs])
  const spanMs = points.length > 1 ? points[points.length - 1]._t - points[0]._t : 0
  const formatTick = tickFormatter ?? defaultTickFormatter(spanMs > DAY_MS)

  return (
    <Card>
      <Section variant="transparent" padding={0} width="100%" height={height}>
        <ResponsiveContainer width="100%" height="100%">
          <ComposedChart
            accessibilityLayer
            data={points}
            margin={{ top: 8, right: 12, left: 0, bottom: 0 }}
          >
            <CartesianGrid stroke="var(--color-border)" strokeDasharray="3 3" />
            <XAxis
              dataKey="_t"
              type="number"
              scale="time"
              domain={['dataMin', 'dataMax']}
              tickFormatter={(t: number) => formatTick(new Date(t).toISOString())}
              stroke="var(--color-text-secondary)"
              fontSize={11}
              tickLine={false}
              axisLine={false}
            />
            <YAxis
              stroke="var(--color-text-secondary)"
              fontSize={11}
              tickLine={false}
              axisLine={false}
              allowDecimals={false}
              width={32}
            />
            <RechartsTooltip
              contentStyle={{
                background: 'var(--color-background-popover)',
                border: '1px solid var(--color-border)',
                borderRadius: 4,
                fontSize: 12,
              }}
              labelStyle={{ color: 'var(--color-text-primary)' }}
              labelFormatter={(t) => new Date(Number(t)).toLocaleString()}
            />
            <Legend
              wrapperStyle={{ fontSize: 12, color: 'var(--color-text-secondary)' }}
              iconType="circle"
            />
            {series.map((s) => (
              <Area
                key={s.key}
                type={curve}
                isAnimationActive={false}
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
      </Section>
      <Stack gap={2}>
        <Text type="supporting">
          {series.map((item) => item.label).join(', ')} over {data.length} recorded time points.
          Times in {TIME_ZONE}.
        </Text>
        <details onToggle={(e) => setTableOpen(e.currentTarget.open)}>
          <summary>View chart data</summary>
          {tableOpen && (
            // Table bleeds into its container's padding when it is a first
            // child (Astryx containerBleed). Inside the Card it slid 12px up
            // over the <summary> and swallowed clicks meant to close it; a
            // zero-padding Section resets the bleed to 0.
            <Section variant="transparent" padding={0} className="min-w-0">
              <Table
                aria-label="Chart data"
                columns={['Time', ...series.map((item) => item.key)].map((key) => ({
                  key,
                  header: key,
                  width: proportional(1),
                }))}
              >
                <TableHeader>
                  <TableRow>
                    <TableHeaderCell>Time</TableHeaderCell>
                    {series.map((item) => (
                      <TableHeaderCell key={item.key}>{item.label}</TableHeaderCell>
                    ))}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data.map((point, index) => (
                    <TableRow key={`${point.ts}-${index}`}>
                      <TableCell>{new Date(point.ts).toLocaleString()}</TableCell>
                      {series.map((item) => (
                        <TableCell key={item.key}>{point[item.key] ?? 'Unavailable'}</TableCell>
                      ))}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Section>
          )}
        </details>
      </Stack>
    </Card>
  )
}
