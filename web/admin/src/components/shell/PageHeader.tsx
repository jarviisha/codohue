import { useContext, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { PageHeaderSlotContext } from './pageHeaderSlot'

/**
 * PageHeader teleports its children into the AppShellHeader slot so every
 * page declares its own title / subtitle / actions inline next to the page
 * content while the chrome lives in the global shell.
 *
 * Usage:
 *
 *   <PageHeader>
 *     <Inline justify="between" align="center" className="w-full">
 *       <Stack gap="025">
 *         <h1>Fleet</h1>
 *         <p>{summary}</p>
 *       </Stack>
 *       <Button>action</Button>
 *     </Inline>
 *   </PageHeader>
 *
 * Renders nothing on the page tree — the children appear inside
 * AppShellHeader instead. When the page unmounts (route change), React
 * cleans up the portal and the next page's PageHeader takes over.
 */
export default function PageHeader({ children }: { children: ReactNode }) {
  const slot = useContext(PageHeaderSlotContext)
  if (!slot) return null
  return createPortal(children, slot)
}
