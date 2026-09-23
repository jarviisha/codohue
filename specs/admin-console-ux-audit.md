# Admin console UX audit

Date: 2026-09-18

## Scope and method

Initial audit and prioritized refactor proposal for daily operators, using the installed `ui-ux-pro-max` skill. Reviewed the shell, route definitions, namespace configuration, catalog and subject lists, event monitoring, health page, chart wrapper, and related query hooks. Skill searches covered error-summary validation, navigation hierarchy, and React forms. Existing Astryx components, neutral theme, and semantic tokens remain the implementation foundation.

This is a source review, not a browser accessibility or visual certification. Frontend dependencies are not installed in this checkout. No application code was changed and no runtime tests were run. Findings below distinguish directly observable source behavior from browser checks still needed.

## Prioritized findings

P1 means lost work, misleading operational information, or broken navigation. P2 means recurring friction or accessibility gaps. P3 means presentation consistency. These are audit priorities, not incident classifications.

### 1. P1 — Background updates can discard configuration drafts

- Evidence: `web/admin/src/pages/ns/config/NamespaceConfigPage.tsx:94` keys `ConfigForm` by `config.updated_at`; `web/admin/src/services/namespaces.ts:130` refreshes the dashboard every 30 seconds.
- Trigger: edit a field, let another operator update the same namespace, then wait for refresh. A changed timestamp remounts the form and replaces local draft state. The route blocker cannot prevent a remount.
- Proposal: scope the editor by namespace; keep a stable editing baseline while dirty, show an external-update notice, and offer an explicit reload/discard action. Track edits independently from the API diff. Review edits made while a save is pending so a successful save does not erase newer input.
- Acceptance: external refresh preserves every unsaved field; navigating to another namespace never carries its predecessor's draft; a successful save establishes the correct new baseline. Preventing concurrent server writes would require a separate backend contract decision.

### 2. P1 — Invalid action-weight rows disappear or overwrite each other

- Evidence: `web/admin/src/pages/ns/config/NamespaceConfigPage.tsx:499` builds a map, skips blank names and non-finite values, and overwrites duplicate trimmed names. Dirty state is derived from that normalized payload.
- Trigger: add two rows named `VIEW` with different weights, or clear an existing action name. Saving can submit fewer actions than the editor shows. A new blank row may not mark the form dirty.
- Proposal: validate raw rows before constructing the request; flag empty/duplicate names and invalid numbers inline. Add a focusable error summary linked to fields and stable row identities.
- Acceptance: invalid rows cannot be silently omitted; duplicate names cannot overwrite one another; unsaved editor changes activate the leave guard.

### 3. P1 — Namespace switching carries string subject IDs across contexts

- Evidence: `web/admin/src/components/shell/NamespaceSwitcher.tsx:32` strips only numeric path segments. Subject IDs are strings in `web/admin/src/App.tsx` and subject list links.
- Trigger: switch namespaces from `/ns/a/subjects/user-42`; the destination remains `/ns/b/subjects/user-42`. That subject may be missing or represent a different person.
- Proposal: derive a safe destination from known route patterns; detail routes switch to the corresponding list. Keep namespace-specific IDs and filters out of the new context by default.
- Acceptance: string, numeric, and encoded subject IDs all return to the Subjects list; catalog and batch-run details return to their lists; ordinary section routes remain in that section.

### 4. P1 — Command-palette dynamic results are absent from selection lookup

- Evidence: `web/admin/src/components/shell/CommandPalette.tsx:77` resolves selections only against static `commands`, while `searchSource.search` adds synthetic `deepLinks` results separately.
- Trigger to verify in-browser: search `#42` or a subject ID, then select the generated result. The application selection callback has no matching command to execute.
- Proposal: execute the selected search result through Astryx's supported selection API or keep a result lookup containing dynamic entries. Also add the missing namespace Configuration command.
- Acceptance: keyboard and pointer selection navigate correctly for static and dynamic entries, close the palette, and respect the unsaved-changes guard. Confirm Astryx callback behavior before implementation.

### 5. P1 — Event-summary failures look like zero traffic

- Evidence: `web/admin/src/pages/ns/events/EventsPage.tsx:340` renders totals with `summary.data?.total ?? 0` and an empty-series message without checking loading/error states.
- Trigger: a slow or failed summary request shows zero events and zero rate. Operators cannot distinguish missing telemetry from inactivity.
- Proposal: separate loading, error, valid empty, and stale-data states. Preserve the last successful snapshot with its age when a refresh fails; provide Retry. Present stream connectivity separately from summary-query status.
- Acceptance: a failed request never presents fabricated zero values; actual zero remains a valid state; retries and last-success timestamps are visible.

### 6. P2 — List context is lost on return and links cannot reproduce the view

- Evidence: `CatalogItemsPage.tsx:52` stores state, search and page locally; only author is URL-backed. `SubjectsListPage.tsx` stores search, sort and page locally too.
- Trigger: filter/page through results, open detail, then navigate back. Remounting resets local list state. Copying the URL omits most of the view configuration.
- Proposal: make validated URL parameters the source of truth for applied filters, sorting and pagination; debounce text searches or use explicit Apply; reset the page when filters or namespace change. Keep previous rows visible with a refresh indicator where appropriate.
- Acceptance: Back/Forward, reload and copied links reproduce the applied view; invalid page parameters recover safely; rapid typing does not issue a request for every character.

### 7. P2 — Configuration form lacks semantic section headings and a descriptive switch label

- Evidence: `NamespaceConfigPage.tsx:261` names the authored-item switch only `on`/`off`; section titles at line 377 use styled spans. Save failures have a top banner but no field-linked error handling.
- Proposal: give the switch a stable name such as “Exclude self-authored items”; use semantic h2 section headings through supported Astryx typography APIs; connect field errors to controls and move focus to an error summary after failed validation. Replace API-oriented subtitles with operator-facing consequences and units.
- Acceptance: heading navigation exposes every form section; the switch's purpose and state are separately available; validation errors are reachable without scanning the entire form.

### 8. P2 — Monitoring controls and data freshness need explicit state

- Evidence: `EventsPage.tsx:348` renders every time-window button with the same primary variant and no selected-state property. `HealthPage.tsx` announces a 30-second refresh interval but no last-success time or manual retry; an error replaces the status view.
- Proposal: use an Astryx single-selection control for the time window; expose its selected state programmatically. Show last successful refresh and refresh failure alongside retained health data. Announce concise state changes without announcing every event row.
- Acceptance: keyboard and screen-reader users can identify and change the active window; unavailable telemetry is distinct from a degraded dependency; manual retry is operable.

### 9. P2 — Table and chart accessibility need a runtime verification pass

- Evidence: catalog error previews are truncated with native `title`; row action labels repeat “Delete”/“Redrive” without item context. `TimeSeriesChart.tsx` supplies no explicit accessible summary or equivalent data view.
- Proposal: name row actions with their object context, provide keyboard/touch access to full error messages, and give charts a concise summary plus an accessible data view. Verify table scrolling, focus visibility, headers and long-ID wrapping with the actual Astryx DOM.
- Acceptance: important error text is available without hover; row actions are distinguishable outside visual row context; chart information has a nonvisual equivalent; narrow screens contain table overflow within its region.
- Limit: the review does not establish what accessibility Astryx or Recharts already supplies internally; inspect rendered output before adding duplicate semantics.

### 10. P3 — Layout and navigation consistency can improve after correctness fixes

- Evidence: the shell uses `Stack padding={6}`, while `PageContainer` defaults to additional padding and Events adds `px-6 py-6`. Status labels use Badge where the local Astryx guide reserves Badge for counts. Sidebar entries use click navigation rather than explicit destinations.
- Proposal: establish one owner for page gutters using Astryx layout primitives; use StatusDot/Token for status; use supported link destinations for navigation so open-in-new-tab works. Group infrequent Demo data/Danger zone destinations separately from everyday monitoring. Preserve stable ordering of global and namespace navigation.
- Acceptance: consistent page gutters across routes and breakpoints; semantic tokens in both themes; native link interactions work; state remains understandable without color.

## Refactor sequence

1. **Correctness:** protect drafts, validate action weights, repair safe namespace switching and palette selection, distinguish missing telemetry from zero traffic.
2. **Daily workflow:** URL-backed list state, stable table refresh, explicit monitoring-window selection, freshness and retry controls.
3. **Accessibility and presentation:** headings, labels, error focus, contextual actions, chart alternatives, consistent Astryx layout/status primitives and operator-facing copy.

Before UI edits, follow the local Astryx discovery workflow (`build`, layout documentation, component APIs). Consult existing tokens instead of generating a replacement palette or typography system. Reuse focused page-state and table-toolbar components only where the pages share real behavior.

## Verification plan for implementation

- Add targeted regressions for draft preservation, action validation, route-context switching, dynamic palette selection and URL-state restoration.
- Exercise initial load, background refresh failure, retry, valid empty results, and stale snapshots with controlled responses.
- Browser walkthrough: Fleet → namespace → filtered list → detail → Back; switch namespace on a detail route; edit configuration during refresh; open palette with keyboard; inspect a failed event-summary request.
- Check keyboard-only operation, dialog focus return, screen-reader labels, 200% zoom, long identifiers, narrow/wide viewports, light/dark themes and reduced-motion preferences.
- Run frontend lint, tests and build; follow repository-required Go checks for submitted code changes, and E2E when implementation affects API behavior or integration flows.

## Implementation follow-up

Implemented on `fix/admin-console-operator-ux` after the initial audit:

- Stable namespace-scoped configuration editor, explicit external-update reload dialog, save-time input locking, raw draft tracking, stable action rows, inline validation and a focused error summary.
- Route-aware namespace switching, dynamic palette selection and Configuration command, native sidebar links and a separate Tools group. Following operator feedback, namespace routes show only namespace navigation plus an “All namespaces” return link; global destinations appear outside the namespace scope.
- URL-backed catalog/subject filters and pagination, explicit search application, retries and stale-snapshot feedback, contextual row actions and keyboard-operable full error details.
- Distinct telemetry loading/error/zero/stale states, refresh timestamps and controls, semantic time-window selection, Astryx status tokens, chart data tables and disabled chart animation.
- Shared shell gutter ownership and explicit table column width budgets, verified visually at narrow and desktop widths. Standalone login/auth screens retain their own padding.
- Unit regressions and a Playwright workflow suite using mocked APIs; a Makefile target and CI browser step make the checks repeatable.

Validation: frontend lint, unit tests and production build; `make test` and
`make test-race`; Chrome browser regressions covering the above workflows,
375/768/1440px catalog layouts, dark mode and narrow configuration layout.
Screenshots were inspected for catalog readability and page containment.
Browser tests use controlled fixtures rather than a live backend integration.
The initial UI refactor did not change API, storage or migration contracts.
A follow-up health fix connects admin to the existing authenticated component
diagnostics endpoint using the shared observability token. Aggregate-only
responses now show component statuses as `unknown`, and diagnostic errors use
the error color. Regression fixtures cover the real aggregate-only response;
the earlier all-components fixture missed this integration mismatch.

Remaining limits: this is not a full assistive-technology or contrast certification.
Server-side optimistic concurrency is outside this UI change; another write can
still race between the latest observed snapshot and a save. Existing bundle-size
warnings remain outside the audit's correctness and operator-workflow scope.
