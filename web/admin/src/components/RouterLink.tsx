import { Link } from 'react-router-dom'
import type { ComponentProps, FocusEvent, MouseEvent } from 'react'
import { preloadRoute } from '@/routes'

type Props = Omit<ComponentProps<typeof Link>, 'to'> & { href: string }

/**
 * Astryx renders links with an `href`; React Router's Link takes a `to`. This
 * adapter bridges the two so <LinkProvider> can hand every Astryx link
 * (Button href, Link, BreadcrumbItem, SideNavItem…) to the router instead of
 * letting it fall back to a full-page navigation.
 *
 * It is also the one place every navigation affordance in the app passes
 * through, which makes it the right place to warm the destination's chunk.
 * Pointing at or tabbing to a link is a reliable signal of intent and starts
 * the download ahead of the click; by the time the click lands the module is
 * usually already resolved and the navigation is immediate. Only the module is
 * fetched — page data stays behind the page's own queries — so hovering a
 * sidebar never issues an API call.
 */
export default function RouterLink({ href, onMouseEnter, onFocus, ...props }: Props) {
  return (
    <Link
      {...props}
      to={href}
      onMouseEnter={(e: MouseEvent<HTMLAnchorElement>) => {
        preloadRoute(href)
        onMouseEnter?.(e)
      }}
      onFocus={(e: FocusEvent<HTMLAnchorElement>) => {
        preloadRoute(href)
        onFocus?.(e)
      }}
    />
  )
}
