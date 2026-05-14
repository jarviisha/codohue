import { Fragment } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { parsePs1, segmentTo } from '@/routes/nav'

// Top-bar location prompt. Renders `codohue@{ns}:~/{segments} $` with `@ns`
// and each path segment as clickable links (per DESIGN.md §3.1.1).
export default function Ps1Prompt() {
  const { pathname } = useLocation()
  const { ns, segments } = parsePs1(pathname)

  return (
    <div className="font-mono text-sm select-none">
      <span className="text-muted">codohue@</span>
      <span className="text-accent">{ns}</span>
      <span className="text-muted">:~</span>
      {segments.map((seg, i) => (
        <Fragment key={`${seg}-${i}`}>
          <span className="text-muted">/</span>
          <Link
            to={segmentTo(ns, segments, i)}
            className="text-primary hover:text-accent"
          >
            {seg}
          </Link>
        </Fragment>
      ))}
      <span className="text-muted"> $</span>
    </div>
  )
}
