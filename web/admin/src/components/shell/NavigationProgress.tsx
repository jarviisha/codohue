import { ProgressBar, Stack } from '@astryxdesign/core'
import useNavigationPending from '@/components/shell/useNavigationPending'

/**
 * NavigationProgress draws the pending state of a route navigation: the window
 * between clicking a destination and its page module being ready to render.
 *
 * The bar carries its own accessible name, so the progressbar role announces
 * where the app is going rather than an anonymous "loading". The status region
 * announces the same thing for assistive tech that is not tracking the bar.
 */
export default function NavigationProgress() {
  const { isSlow, to } = useNavigationPending()

  if (!isSlow || !to) return null

  return (
    <Stack role="status" aria-live="polite">
      <ProgressBar
        label={`Loading ${describeDestination(to)}`}
        isLabelHidden
        isIndeterminate
        variant="accent"
      />
    </Stack>
  )
}

/**
 * describeDestination turns a pathname into something worth announcing. The
 * trailing segment names the page in every route the shell owns —
 * /system/runtime reads "runtime", /ns/bluesky/events reads "events" — and a
 * bare "/" is the fleet overview.
 */
function describeDestination(pathname: string): string {
  const segments = pathname.split('/').filter(Boolean)
  if (segments.length === 0) return 'fleet overview'
  const last = segments[segments.length - 1]
  return /^\d+$/.test(last) ? `#${last}` : last.replace(/-/g, ' ')
}
