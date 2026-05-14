import type { HTMLAttributes, ReactNode, TdHTMLAttributes, ThHTMLAttributes } from 'react'

// Dense ops table. Outer wrapper handles horizontal overflow so wide column
// sets don't bust the layout. See DESIGN.md §9.

export function Table({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-sm overflow-x-auto bg-surface">
      <table className="w-full border-collapse text-sm leading-5">{children}</table>
    </div>
  )
}

export function Thead({ children }: { children: ReactNode }) {
  return <thead>{children}</thead>
}

export function Tbody({ children }: { children: ReactNode }) {
  return <tbody>{children}</tbody>
}

interface TrProps extends HTMLAttributes<HTMLTableRowElement> {
  children: ReactNode
}

export function Tr({ children, className = '', ...rest }: TrProps) {
  return (
    <tr
      className={`border-b border-default last:border-b-0 hover:bg-surface-raised ${className}`}
      {...rest}
    >
      {children}
    </tr>
  )
}

interface ThProps extends ThHTMLAttributes<HTMLTableCellElement> {
  align?: 'left' | 'right' | 'center'
  children: ReactNode
}

const ALIGN: Record<'left' | 'right' | 'center', string> = {
  left:   'text-left',
  right:  'text-right',
  center: 'text-center',
}

export function Th({ align = 'left', className = '', children, ...rest }: ThProps) {
  return (
    <th
      scope="col"
      className={`px-3 py-2.5 font-mono text-xs uppercase tracking-[0.04em] text-secondary ${ALIGN[align]} ${className}`}
      {...rest}
    >
      {children}
    </th>
  )
}

interface TdProps extends TdHTMLAttributes<HTMLTableCellElement> {
  align?: 'left' | 'right' | 'center'
  mono?: boolean
  children?: ReactNode
}

export function Td({ align = 'left', mono = false, className = '', children, ...rest }: TdProps) {
  return (
    <td
      className={`px-3 py-2.5 text-secondary ${ALIGN[align]} ${mono ? 'font-mono tabular-nums text-primary' : ''} ${className}`}
      {...rest}
    >
      {children}
    </td>
  )
}
