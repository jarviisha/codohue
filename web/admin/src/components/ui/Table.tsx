import type { ReactNode, TdHTMLAttributes, ThHTMLAttributes } from 'react'

export function Table({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <table className={['w-full border-collapse', className].filter(Boolean).join(' ')}>
      {children}
    </table>
  )
}

export function Thead({ children }: { children: ReactNode }) {
  return (
    <thead>
      <tr className="border-b border-default">{children}</tr>
    </thead>
  )
}

export function Th({
  children,
  className,
  align = 'left',
  ...props
}: {
  children?: ReactNode
  className?: string
  align?: 'left' | 'right' | 'center'
} & Omit<ThHTMLAttributes<HTMLTableCellElement>, 'className' | 'align'>) {
  return (
    <th
      className={[
        'text-[11px] font-semibold uppercase tracking-[0.06em] text-muted pb-2.5',
        align === 'right' ? 'text-right' : align === 'center' ? 'text-center' : 'text-left',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
      {...props}
    >
      {children}
    </th>
  )
}

export function Tbody({ children }: { children: ReactNode }) {
  return <tbody>{children}</tbody>
}

export function Tr({
  children,
  className,
  hoverable,
  ...props
}: {
  children: ReactNode
  className?: string
  hoverable?: boolean
  onClick?: () => void
}) {
  return (
    <tr
      className={[
        'border-b border-default last:border-0',
        hoverable && 'hover:bg-surface-raised',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
      {...props}
    >
      {children}
    </tr>
  )
}

export function Td({
  children,
  className,
  align,
  mono,
  muted,
  ...props
}: {
  children?: ReactNode
  className?: string
  align?: 'left' | 'right' | 'center'
  mono?: boolean
  muted?: boolean
} & Omit<TdHTMLAttributes<HTMLTableCellElement>, 'className' | 'align'>) {
  return (
    <td
      className={[
        'py-2.5 text-sm',
        muted ? 'text-muted' : 'text-primary',
        align === 'right' ? 'text-right' : align === 'center' ? 'text-center' : '',
        mono ? 'font-mono tabular-nums' : '',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
      {...props}
    >
      {children}
    </td>
  )
}
