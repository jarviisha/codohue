import { useState, type ReactNode } from 'react'
import {
  Badge,
  Button,
  Checkbox,
  CodeBadge,
  CodeBlock,
  ConfirmDialog,
  Dropdown,
  DropdownItem,
  EmptyState,
  Field,
  Form,
  FormGrid,
  Input,
  Kbd,
  KeyValueList,
  LoadingState,
  MetricTile,
  Modal,
  Notice,
  NumberInput,
  PageHeader,
  PageShell,
  Pagination,
  Panel,
  RadioGroup,
  Select,
  StatusToken,
  Switch,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Toolbar,
  Tr,
} from '../../components/ui'

// /_kitchen-sink — preview every primitive in src/components/ui with state
// variants. Not part of operator nav; reach it by typing the URL during dev.

// ────────────────────────────────────────────────────────────────────────────
// TOC structure
// ────────────────────────────────────────────────────────────────────────────

type TocCategory = { id: string; name: string; entries: { id: string; name: string }[] }

const TOC: TocCategory[] = [
  {
    id: 'layout',
    name: 'Layout',
    entries: [
      { id: 'pageshell', name: 'PageShell' },
      { id: 'pageheader', name: 'PageHeader' },
    ],
  },
  {
    id: 'content',
    name: 'Content',
    entries: [
      { id: 'panel', name: 'Panel' },
      { id: 'toolbar', name: 'Toolbar' },
      { id: 'table', name: 'Table' },
      { id: 'metrictile', name: 'MetricTile' },
      { id: 'badge', name: 'Badge' },
      { id: 'keyvaluelist', name: 'KeyValueList' },
      { id: 'codebadge', name: 'CodeBadge' },
      { id: 'emptystate', name: 'EmptyState' },
      { id: 'loadingstate', name: 'LoadingState' },
      { id: 'notice', name: 'Notice' },
    ],
  },
  {
    id: 'forms',
    name: 'Forms',
    entries: [
      { id: 'input', name: 'Input' },
      { id: 'select', name: 'Select' },
      { id: 'numberinput', name: 'NumberInput' },
      { id: 'checkbox', name: 'Checkbox' },
      { id: 'radio', name: 'Radio + RadioGroup' },
      { id: 'switch', name: 'Switch' },
      { id: 'form', name: 'Form + FormGrid + Field' },
    ],
  },
  {
    id: 'overlays',
    name: 'Overlays',
    entries: [
      { id: 'button', name: 'Button' },
      { id: 'modal', name: 'Modal' },
      { id: 'confirmdialog', name: 'ConfirmDialog' },
      { id: 'dropdown', name: 'Dropdown' },
      { id: 'commandpalette', name: 'CommandPalette' },
    ],
  },
  {
    id: 'status',
    name: 'Status',
    entries: [
      { id: 'statustoken', name: 'StatusToken' },
      { id: 'kbd', name: 'Kbd' },
    ],
  },
  {
    id: 'navigation',
    name: 'Navigation',
    entries: [{ id: 'pagination', name: 'Pagination' }],
  },
  {
    id: 'data',
    name: 'Data display',
    entries: [{ id: 'codeblock', name: 'CodeBlock' }],
  },
]

// ────────────────────────────────────────────────────────────────────────────
// Section helpers
// ────────────────────────────────────────────────────────────────────────────

function CategoryHeader({ id, name }: { id: string; name: string }) {
  return (
    <h2
      id={id}
      className="scroll-mt-16 font-mono text-[11px] uppercase tracking-[0.12em] text-muted mt-10 first:mt-0 border-b border-default pb-2"
    >
      {name}
    </h2>
  )
}

interface ComponentEntryProps {
  id: string
  name: string
  hint?: string
  children: ReactNode
}

function ComponentEntry({ id, name, hint, children }: ComponentEntryProps) {
  return (
    <section id={id} className="scroll-mt-16">
      <header className="mb-3">
        <h3 className="text-sm font-semibold text-primary">{name}</h3>
        {hint ? <p className="text-xs text-muted mt-0.5">{hint}</p> : null}
      </header>
      <div className="flex flex-col gap-3">{children}</div>
    </section>
  )
}

// Small wrapper for showing a primitive's variants in a labelled inline row.
function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-center gap-3">
      <span className="font-mono text-[11px] uppercase tracking-[0.06em] text-muted w-24 shrink-0">
        {label}
      </span>
      <div className="flex items-center gap-2 flex-wrap">{children}</div>
    </div>
  )
}

// ────────────────────────────────────────────────────────────────────────────
// Page
// ────────────────────────────────────────────────────────────────────────────

export default function KitchenSinkPage() {
  // overlays
  const [modalOpen, setModalOpen] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)

  // forms
  const [name, setName] = useState('')
  const [checkOn, setCheckOn] = useState(true)
  const [autoEmbed, setAutoEmbed] = useState(true)
  const [liveTail, setLiveTail] = useState(false)
  const [strategy, setStrategy] = useState<'item2vec' | 'svd' | 'byoe' | 'disabled'>(
    'item2vec',
  )

  // table + pagination
  const [bulk, setBulk] = useState<Record<string, boolean>>({ r1: true, r2: false })
  const [offset, setOffset] = useState(0)
  const limit = 25
  const total = 137

  return (
    <PageShell>
      <PageHeader
        title="Kitchen sink"
        meta="every shared primitive · /_kitchen-sink"
        actions={
          <Button
            variant="secondary"
            size="sm"
            onClick={() =>
              window.scrollTo({ top: 0, behavior: 'smooth' })
            }
          >
            top
          </Button>
        }
      />

      <div className="grid grid-cols-1 lg:grid-cols-[12rem_1fr] gap-6">
        {/* ─── TOC ─── */}
        <aside className="lg:sticky lg:top-16 lg:self-start lg:max-h-[calc(100vh-5rem)] lg:overflow-y-auto">
          <nav className="flex flex-col gap-5 text-sm">
            {TOC.map((cat) => (
              <div key={cat.id} className="flex flex-col gap-1">
                <a
                  href={`#${cat.id}`}
                  className="font-mono text-[11px] uppercase tracking-[0.12em] text-muted hover:text-primary"
                >
                  {cat.name}
                </a>
                <ul className="flex flex-col gap-0.5 pl-1">
                  {cat.entries.map((e) => (
                    <li key={e.id}>
                      <a
                        href={`#${e.id}`}
                        className="block py-0.5 text-secondary hover:text-primary"
                      >
                        {e.name}
                      </a>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </nav>
        </aside>

        {/* ─── Sections ─── */}
        <div className="flex flex-col gap-10 min-w-0">
          {/* ──────── Layout ──────── */}
          <CategoryHeader id="layout" name="Layout" />

          <ComponentEntry
            id="pageshell"
            name="PageShell"
            hint="Vertical-rhythm wrapper. The grid you're reading sits inside one."
          >
            <Panel>
              <p className="text-sm text-secondary">
                This whole route is wrapped in <CodeBadge>&lt;PageShell&gt;</CodeBadge>.
                It only adds <CodeBadge>flex flex-col gap-4</CodeBadge> between its
                direct children — no visible chrome.
              </p>
            </Panel>
          </ComponentEntry>

          <ComponentEntry
            id="pageheader"
            name="PageHeader"
            hint="title · meta · right-aligned actions"
          >
            <Panel>
              <PageHeader
                title="Sample page"
                meta="last refreshed 14:02:38 UTC"
                actions={
                  <>
                    <Button variant="ghost" size="sm">
                      Refresh
                    </Button>
                    <Button variant="primary" size="sm">
                      Primary
                    </Button>
                  </>
                }
              />
            </Panel>
          </ComponentEntry>

          {/* ──────── Content ──────── */}
          <CategoryHeader id="content" name="Content" />

          <ComponentEntry
            id="panel"
            name="Panel"
            hint="Bordered surface with optional title / actions / footer."
          >
            <Panel
              title="With title and actions"
              actions={<Badge tone="accent">live</Badge>}
              footer={<span>Footer line · click anywhere to test borders</span>}
            >
              <p className="text-sm text-secondary">
                Body content. No nested decorative cards.
              </p>
            </Panel>
            <Panel>
              <p className="text-sm text-secondary">
                Panel without a title — content sits flush at <CodeBadge>p-4</CodeBadge>.
              </p>
            </Panel>
          </ComponentEntry>

          <ComponentEntry
            id="toolbar"
            name="Toolbar"
            hint="Compact filter + action row. Children sit on h-7 controls."
          >
            <Toolbar>
              <Field label="state" htmlFor="ks-tb-state">
                <Select id="ks-tb-state" selectSize="sm" defaultValue="all">
                  <option>all</option>
                  <option>ok</option>
                  <option>fail</option>
                </Select>
              </Field>
              <Field label="object id" htmlFor="ks-tb-objid">
                <Input id="ks-tb-objid" inputSize="sm" placeholder="sku_" />
              </Field>
              <div className="ml-auto flex items-center gap-2">
                <Button variant="secondary" size="sm">
                  Refresh
                </Button>
                <Button variant="primary" size="sm">
                  Bulk action
                </Button>
              </div>
            </Toolbar>
          </ComponentEntry>

          <ComponentEntry
            id="table"
            name="Table"
            hint="Table family (Table / Thead / Tbody / Tr / Th / Td). Numeric columns get align=right + mono."
          >
            <Table>
              <Thead>
                <Tr>
                  <Th>state</Th>
                  <Th>object id</Th>
                  <Th>updated</Th>
                  <Th align="right">attempts</Th>
                </Tr>
              </Thead>
              <Tbody>
                <Tr>
                  <Td>
                    <StatusToken state="ok" />
                  </Td>
                  <Td mono>sku_4291</Td>
                  <Td>14:02 today</Td>
                  <Td mono align="right">
                    1
                  </Td>
                </Tr>
                <Tr>
                  <Td>
                    <StatusToken state="run" />
                  </Td>
                  <Td mono>sku_4289</Td>
                  <Td>14:01 today</Td>
                  <Td mono align="right">
                    1
                  </Td>
                </Tr>
                <Tr>
                  <Td>
                    <StatusToken state="fail" />
                  </Td>
                  <Td mono>sku_4042</Td>
                  <Td>13:55 today</Td>
                  <Td mono align="right">
                    5
                  </Td>
                </Tr>
              </Tbody>
            </Table>
          </ComponentEntry>

          <ComponentEntry
            id="metrictile"
            name="MetricTile"
            hint="Uniform-size summary tile. No hero variant."
          >
            <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
              <MetricTile label="events" value="12,418" hint="last 24h" />
              <MetricTile label="subjects" value="1,204" />
              <MetricTile label="objects" value="4,891" />
              <MetricTile label="dead letter" value="0" hint="catalog backlog" />
            </div>
          </ComponentEntry>

          <ComponentEntry
            id="badge"
            name="Badge"
            hint="Non-status tags (trigger source, TTL, hint). Status uses StatusToken."
          >
            <Row label="tones">
              <Badge>neutral</Badge>
              <Badge tone="accent">accent</Badge>
            </Row>
          </ComponentEntry>

          <ComponentEntry
            id="keyvaluelist"
            name="KeyValueList"
            hint="Inline definition rows. Labels left mono, values right mono tabular-nums."
          >
            <Panel>
              <KeyValueList
                rows={[
                  { label: 'strategy', value: 'item2vec' },
                  { label: 'dim', value: '128' },
                  { label: 'catalog auto-embed', value: 'enabled' },
                  { label: 'catalog backlog', value: '0' },
                ]}
              />
            </Panel>
          </ComponentEntry>

          <ComponentEntry
            id="codebadge"
            name="CodeBadge"
            hint="Inline mono ID / token pill."
          >
            <p className="text-sm text-secondary">
              Subject <CodeBadge>user_19283</CodeBadge> last viewed{' '}
              <CodeBadge>sku_4291</CodeBadge> at{' '}
              <CodeBadge>14:02:38 UTC</CodeBadge>.
            </p>
          </ComponentEntry>

          <ComponentEntry
            id="emptystate"
            name="EmptyState"
            hint="Dashed-border block for no-data sections."
          >
            <EmptyState
              title="No items yet"
              description="Catalog ingest hasn't produced any items for this namespace."
              action={<Button variant="secondary">Open catalog config</Button>}
            />
          </ComponentEntry>

          <ComponentEntry
            id="loadingstate"
            name="LoadingState"
            hint="Shimmer skeleton (prefer over a full-page spinner)."
          >
            <Panel>
              <LoadingState rows={4} />
            </Panel>
          </ComponentEntry>

          <ComponentEntry
            id="notice"
            name="Notice"
            hint="4px left border + status text; transparent background (DESIGN.md §6.1)."
          >
            <Notice tone="ok" title="Notice — ok">
              Health probe completed.
            </Notice>
            <Notice tone="warn" title="Notice — warn" onDismiss={() => undefined}>
              Embedder ran with degraded latency — investigating.
            </Notice>
            <Notice tone="fail" title="Notice — fail">
              Qdrant heartbeat missed at <CodeBadge>14:02:38 UTC</CodeBadge>.
            </Notice>
            <Notice tone="info">
              Informational notice skips the leading StatusToken.
            </Notice>
          </ComponentEntry>

          {/* ──────── Forms ──────── */}
          <CategoryHeader id="forms" name="Forms" />

          <ComponentEntry
            id="input"
            name="Input"
            hint="Text input. inputSize sm (h-7) for toolbars, md (h-8) default."
          >
            <Row label="md / default">
              <Input placeholder="default" />
              <Input value="filled" readOnly />
              <Input placeholder="invalid" invalid defaultValue="x" />
              <Input placeholder="disabled" disabled />
            </Row>
            <Row label="sm">
              <Input inputSize="sm" placeholder="default" />
              <Input inputSize="sm" placeholder="invalid" invalid defaultValue="x" />
              <Input inputSize="sm" placeholder="disabled" disabled />
            </Row>
          </ComponentEntry>

          <ComponentEntry
            id="select"
            name="Select"
            hint="Native <select>. selectSize sm / md."
          >
            <Row label="md / default">
              <Select defaultValue="all">
                <option value="all">all</option>
                <option value="ok">ok</option>
                <option value="fail">fail</option>
              </Select>
              <Select defaultValue="all" invalid>
                <option value="all">invalid</option>
              </Select>
              <Select defaultValue="all" disabled>
                <option value="all">disabled</option>
              </Select>
            </Row>
            <Row label="sm">
              <Select selectSize="sm" defaultValue="all">
                <option>all</option>
                <option>ok</option>
                <option>fail</option>
              </Select>
            </Row>
          </ComponentEntry>

          <ComponentEntry
            id="numberinput"
            name="NumberInput"
            hint="Mono tabular-nums, right-aligned, fixed width."
          >
            <Row label="widths">
              <NumberInput defaultValue={128} />
              <NumberInput defaultValue={0.7} step={0.05} min={0} max={1} width="w-20" />
              <NumberInput defaultValue={12} invalid />
              <NumberInput defaultValue={5} disabled />
            </Row>
          </ComponentEntry>

          <ComponentEntry
            id="checkbox"
            name="Checkbox"
            hint="Native checkbox tinted via accent-color. Indeterminate set imperatively."
          >
            <Row label="states">
              <label className="inline-flex items-center gap-2 cursor-pointer">
                <Checkbox
                  checked={checkOn}
                  onChange={(e) => setCheckOn(e.target.checked)}
                />
                <span className="text-sm text-primary">controlled</span>
              </label>
              <label className="inline-flex items-center gap-2 cursor-pointer">
                <Checkbox indeterminate />
                <span className="text-sm text-primary">indeterminate</span>
              </label>
              <label className="inline-flex items-center gap-2 opacity-50">
                <Checkbox disabled />
                <span className="text-sm text-primary">disabled</span>
              </label>
            </Row>
          </ComponentEntry>

          <ComponentEntry
            id="radio"
            name="Radio + RadioGroup"
            hint="Native radio (also tinted). RadioGroup renders vertical labels + hints + per-option disabled."
          >
            <Panel>
              <Field label="Dense strategy" htmlFor="ks-strategy-radio">
                <RadioGroup<'item2vec' | 'svd' | 'byoe' | 'disabled'>
                  name="ks-strategy-radio"
                  value={strategy}
                  onChange={setStrategy}
                  options={[
                    {
                      value: 'item2vec',
                      label: 'item2vec',
                      hint: 'Trained item embeddings via skip-gram on co-occurrence',
                    },
                    {
                      value: 'svd',
                      label: 'svd',
                      hint: 'Truncated SVD over the interaction matrix',
                    },
                    {
                      value: 'byoe',
                      label: 'byoe',
                      hint: 'Bring-your-own-embeddings via PUT /objects/:id/embedding',
                    },
                    {
                      value: 'disabled',
                      label: 'disabled',
                      hint: 'Skip the dense phase entirely',
                      disabled: true,
                    },
                  ]}
                />
              </Field>
            </Panel>
          </ComponentEntry>

          <ComponentEntry
            id="switch"
            name="Switch"
            hint="<button role=switch>, CSS-only knob. Pair with an explicit ariaLabel."
          >
            <Panel>
              <div className="flex flex-col gap-2">
                <div className="flex items-center justify-between text-sm">
                  <span className="text-primary">Catalog auto-embed</span>
                  <Switch
                    checked={autoEmbed}
                    onChange={setAutoEmbed}
                    ariaLabel="Toggle catalog auto-embed"
                  />
                </div>
                <div className="flex items-center justify-between text-sm">
                  <span className="text-primary">Live tail</span>
                  <Switch
                    checked={liveTail}
                    onChange={setLiveTail}
                    ariaLabel="Toggle live tail"
                  />
                </div>
                <div className="flex items-center justify-between text-sm opacity-50">
                  <span className="text-primary">Disabled switch</span>
                  <Switch
                    checked={false}
                    onChange={() => undefined}
                    disabled
                    ariaLabel="Disabled example"
                  />
                </div>
              </div>
            </Panel>
          </ComponentEntry>

          <ComponentEntry
            id="form"
            name="Form + FormGrid + Field"
            hint="Vertical Form, FormGrid for paired rows, Field wraps each input with label + hint/error."
          >
            <Panel>
              <Form>
                <FormGrid columns={2}>
                  <Field label="namespace name" htmlFor="ks-form-name" required>
                    <Input
                      id="ks-form-name"
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      placeholder="prod"
                    />
                  </Field>
                  <Field
                    label="embedding dim"
                    htmlFor="ks-form-dim"
                    hint="match your strategy output"
                  >
                    <NumberInput id="ks-form-dim" defaultValue={128} />
                  </Field>
                </FormGrid>
                <FormGrid columns={2}>
                  <Field label="alpha" htmlFor="ks-form-alpha" hint="0.0 — 1.0">
                    <NumberInput
                      id="ks-form-alpha"
                      defaultValue={0.7}
                      step={0.05}
                      min={0}
                      max={1}
                    />
                  </Field>
                  <Field
                    label="api key (invalid example)"
                    htmlFor="ks-form-key"
                    error="key must be at least 16 chars"
                  >
                    <Input id="ks-form-key" invalid defaultValue="short" />
                  </Field>
                </FormGrid>
                <div className="flex justify-end gap-2 mt-1">
                  <Button variant="ghost">Cancel</Button>
                  <Button variant="primary" type="submit">
                    Save
                  </Button>
                </div>
              </Form>
            </Panel>
          </ComponentEntry>

          {/* ──────── Overlays ──────── */}
          <CategoryHeader id="overlays" name="Overlays" />

          <ComponentEntry
            id="button"
            name="Button"
            hint="4 variants × 3 sizes. loading swaps leading icon for a spinner, keeps width."
          >
            <Row label="primary">
              <Button variant="primary" size="sm">
                sm
              </Button>
              <Button variant="primary">md</Button>
              <Button variant="primary" size="lg">
                lg
              </Button>
              <Button variant="primary" loading>
                loading
              </Button>
              <Button variant="primary" disabled>
                disabled
              </Button>
            </Row>
            <Row label="secondary">
              <Button variant="secondary" size="sm">
                sm
              </Button>
              <Button variant="secondary">md</Button>
              <Button variant="secondary" size="lg">
                lg
              </Button>
              <Button variant="secondary" loading>
                loading
              </Button>
              <Button variant="secondary" disabled>
                disabled
              </Button>
            </Row>
            <Row label="ghost">
              <Button variant="ghost" size="sm">
                sm
              </Button>
              <Button variant="ghost">md</Button>
              <Button variant="ghost" size="lg">
                lg
              </Button>
              <Button variant="ghost" disabled>
                disabled
              </Button>
            </Row>
            <Row label="danger">
              <Button variant="danger" size="sm">
                sm
              </Button>
              <Button variant="danger">md</Button>
              <Button variant="danger" size="lg">
                lg
              </Button>
              <Button variant="danger" disabled>
                disabled
              </Button>
            </Row>
          </ComponentEntry>

          <ComponentEntry
            id="modal"
            name="Modal"
            hint="80ms opacity snap, Esc + backdrop close, no focus trap yet (Phase 3)."
          >
            <Button variant="secondary" onClick={() => setModalOpen(true)}>
              Open modal
            </Button>
          </ComponentEntry>

          <ComponentEntry
            id="confirmdialog"
            name="ConfirmDialog"
            hint="Destructive-action gate. destructive prop swaps confirm button to danger."
          >
            <Button variant="danger" onClick={() => setConfirmOpen(true)}>
              Open confirm
            </Button>
          </ComponentEntry>

          <ComponentEntry
            id="dropdown"
            name="Dropdown"
            hint="Click trigger; outside-click + Esc close. DropdownItem(destructive) for destructive options."
          >
            <Dropdown
              trigger={
                <span className="inline-flex items-center gap-1 h-8 px-3 rounded-sm border border-default text-sm text-primary hover:bg-surface-raised">
                  Actions
                </span>
              }
            >
              {(close) => (
                <>
                  <DropdownItem onSelect={close}>Edit</DropdownItem>
                  <DropdownItem onSelect={close}>Duplicate</DropdownItem>
                  <DropdownItem onSelect={close} destructive>
                    Delete
                  </DropdownItem>
                </>
              )}
            </Dropdown>
          </ComponentEntry>

          <ComponentEntry
            id="commandpalette"
            name="CommandPalette"
            hint="Mounted globally in AppShell. Open with Cmd+K (or Ctrl+K)."
          >
            <p className="text-sm text-secondary">
              Press <Kbd>Cmd+K</Kbd> to open. Registry is empty on this page — real
              commands land per route via <CodeBadge>useRegisterCommand</CodeBadge>.
            </p>
          </ComponentEntry>

          {/* ──────── Status ──────── */}
          <CategoryHeader id="status" name="Status" />

          <ComponentEntry
            id="statustoken"
            name="StatusToken"
            hint="6-char dmesg bracket. [ RUN] uses the pulse-run animation."
          >
            <Row label="states">
              {(['ok', 'run', 'idle', 'warn', 'fail', 'pend'] as const).map((s) => (
                <span key={s} className="inline-flex items-center gap-2 text-sm">
                  <StatusToken state={s} />
                  <span className="text-muted">{s}</span>
                </span>
              ))}
            </Row>
          </ComponentEntry>

          <ComponentEntry
            id="kbd"
            name="Kbd"
            hint="Bordered mono keycap for shortcut hints."
          >
            <Row label="examples">
              <Kbd>Cmd+K</Kbd>
              <Kbd>Esc</Kbd>
              <Kbd>Enter</Kbd>
              <Kbd>Shift+Tab</Kbd>
            </Row>
          </ComponentEntry>

          {/* ──────── Navigation ──────── */}
          <CategoryHeader id="navigation" name="Navigation" />

          <ComponentEntry
            id="pagination"
            name="Pagination"
            hint="Offset/limit footer. Plain prev/next labels — no arrow glyphs."
          >
            <Panel>
              <Table>
                <Thead>
                  <Tr>
                    <Th>
                      <Checkbox
                        checked={bulk.r1 && bulk.r2}
                        indeterminate={
                          (bulk.r1 || bulk.r2) && !(bulk.r1 && bulk.r2)
                        }
                        onChange={(e) =>
                          setBulk({ r1: e.target.checked, r2: e.target.checked })
                        }
                        aria-label="Select all rows"
                      />
                    </Th>
                    <Th>state</Th>
                    <Th>object id</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  <Tr>
                    <Td>
                      <Checkbox
                        checked={bulk.r1}
                        onChange={(e) =>
                          setBulk((p) => ({ ...p, r1: e.target.checked }))
                        }
                      />
                    </Td>
                    <Td>
                      <StatusToken state="ok" />
                    </Td>
                    <Td mono>sku_demo_001</Td>
                  </Tr>
                  <Tr>
                    <Td>
                      <Checkbox
                        checked={bulk.r2}
                        onChange={(e) =>
                          setBulk((p) => ({ ...p, r2: e.target.checked }))
                        }
                      />
                    </Td>
                    <Td>
                      <StatusToken state="fail" />
                    </Td>
                    <Td mono>sku_demo_002</Td>
                  </Tr>
                </Tbody>
              </Table>
              <div className="mt-3">
                <Pagination
                  offset={offset}
                  limit={limit}
                  total={total}
                  onOffsetChange={setOffset}
                />
              </div>
            </Panel>
          </ComponentEntry>

          {/* ──────── Data display ──────── */}
          <CategoryHeader id="data" name="Data display" />

          <ComponentEntry
            id="codeblock"
            name="CodeBlock"
            hint="Mono pre/code viewer. Optional language label header + copy-to-clipboard text button."
          >
            <CodeBlock language="json" copyable maxHeight="16rem">
{`{
  "id": "sku_4291",
  "namespace": "prod",
  "state": "embedded",
  "metadata": {
    "category": "shoes",
    "brand": "acme",
    "price_cents": 5999,
    "tags": ["new", "sale"]
  },
  "embedded_at": "2026-05-13T07:02:38.412Z"
}`}
            </CodeBlock>
            <CodeBlock>
{`# minimal — no header strip
$ codohue admin run-batch --namespace prod
[ OK ] cron #1847 done in 4.812s`}
            </CodeBlock>
          </ComponentEntry>
        </div>
      </div>

      {/* ─── Overlays mounted at page root ─── */}
      <Modal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        title="Modal example"
        footer={
          <>
            <Button variant="ghost" onClick={() => setModalOpen(false)}>
              Cancel
            </Button>
            <Button variant="primary" onClick={() => setModalOpen(false)}>
              OK
            </Button>
          </>
        }
      >
        <p className="text-sm text-secondary">
          80ms opacity snap, no translate. Esc and backdrop click close it (DESIGN.md §11).
        </p>
      </Modal>

      <ConfirmDialog
        open={confirmOpen}
        title="Delete namespace?"
        description="This drops all events and embeddings. Cannot be undone."
        confirmLabel="Delete"
        destructive
        onConfirm={() => setConfirmOpen(false)}
        onCancel={() => setConfirmOpen(false)}
      />
    </PageShell>
  )
}
