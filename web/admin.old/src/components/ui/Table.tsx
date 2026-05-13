import type { ReactNode, TdHTMLAttributes, ThHTMLAttributes } from 'react'

export function Table({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <table className={['w-full min-w-full border-separate border-spacing-0 text-left', className].filter(Boolean).join(' ')}>
      {children}
    </table>
  )
}

export function Thead({ children }: { children: ReactNode }) {
  return (
    <thead>
      <tr>{children}</tr>
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
        'border-b border-default px-2.5 py-2 text-[11px] font-semibold uppercase tracking-[0.06em] text-muted first:rounded-tl last:rounded-tr',
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
        'group [&:last-child>td]:border-b-0',
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
        'border-b border-default/80 px-2.5 py-2 text-sm transition-colors duration-100',
        muted ? 'text-muted' : 'text-secondary group-hover:text-primary',
        align === 'right' ? 'text-right' : align === 'center' ? 'text-center' : '',
        mono ? 'tabular-nums' : '',
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
