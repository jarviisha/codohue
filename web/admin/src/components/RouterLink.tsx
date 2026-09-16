import { Link } from 'react-router-dom'
import type { ComponentProps } from 'react'

/**
 * Astryx renders links with an `href`; React Router's Link takes a `to`. This
 * adapter bridges the two so <LinkProvider> can hand every Astryx link
 * (Button href, Link, BreadcrumbItem, SideNavItem…) to the router instead of
 * letting it fall back to a full-page navigation.
 */
export default function RouterLink({
  href,
  ...props
}: Omit<ComponentProps<typeof Link>, 'to'> & { href: string }) {
  return <Link to={href} {...props} />
}
