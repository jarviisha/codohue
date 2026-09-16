import { Section } from '@astryxdesign/core'
import type { ReactNode } from 'react'

/**
 * Astryx has no page-width container: AppShell owns the frame and Section owns
 * a region, but neither caps the reading width or centres itself. Every admin
 * page wants both, so they go here once instead of repeating maxWidth +
 * mx-auto at ~20 call sites.
 *
 * Sizes mirror the widths the pages were already built against.
 */
const MAX_WIDTH = {
  sm: '40rem',
  md: '56rem',
  lg: '72rem',
  xl: '80rem',
  full: 'none',
} as const

export type PageContainerSize = keyof typeof MAX_WIDTH

export default function PageContainer({
  size = 'lg',
  padding = 6,
  className,
  children,
}: {
  size?: PageContainerSize
  /** Astryx spacing step (4px base); 6 = 24px, matching the old page padding. */
  padding?: 0 | 4 | 6 | 8 | 10
  className?: string
  children: ReactNode
}) {
  return (
    <Section
      variant="transparent"
      padding={padding}
      maxWidth={MAX_WIDTH[size]}
      className={className ? `mx-auto w-full ${className}` : 'mx-auto w-full'}
    >
      {children}
    </Section>
  )
}
