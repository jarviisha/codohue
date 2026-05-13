import { useState } from 'react'
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

// Showcase route for every shared primitive. Use during local dev to verify
// visual changes against the design contract end-to-end. Not part of the
// production nav.
export default function KitchenSinkPage() {
  const [modalOpen, setModalOpen] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [name, setName] = useState('')

  // State for new Phase 1.6 primitives
  const [autoEmbed, setAutoEmbed] = useState(true)
  const [liveTail, setLiveTail] = useState(false)
  const [bulkAll, setBulkAll] = useState(false)
  const [bulkRow, setBulkRow] = useState<Record<string, boolean>>({ r1: true, r2: false })
  const [strategy, setStrategy] = useState<'item2vec' | 'svd' | 'byoe' | 'disabled'>('item2vec')
  const [offset, setOffset] = useState(0)
  const limit = 25
  const total = 137

  return (
    <PageShell>
      <PageHeader
        title="Kitchen sink"
        meta="every shared primitive, one place"
        actions={
          <>
            <Button variant="ghost">Ghost</Button>
            <Button variant="secondary">Secondary</Button>
            <Button variant="primary">Primary action</Button>
          </>
        }
      />

      <Notice tone="ok" title="Notice — ok">
        4px left border, status token prefix, no bg fill (DESIGN.md §6.1).
      </Notice>
      <Notice tone="warn" title="Notice — warn" onDismiss={() => undefined}>
        Dismiss action lives in the top-right corner.
      </Notice>
      <Notice tone="fail" title="Notice — fail">
        Body text uses <CodeBadge>text-primary</CodeBadge> for full legibility.
      </Notice>
      <Notice tone="info">
        Informational notice skips the leading <CodeBadge>StatusToken</CodeBadge>.
      </Notice>

      <Panel title="Status tokens">
        <ul className="flex flex-wrap gap-x-6 gap-y-2 text-sm">
          {(['ok', 'run', 'idle', 'warn', 'fail', 'pend'] as const).map((s) => (
            <li key={s} className="flex items-center gap-2">
              <StatusToken state={s} /> <span className="text-muted">{s}</span>
            </li>
          ))}
        </ul>
      </Panel>

      <Panel title="Metric tiles" actions={<Badge tone="accent">live</Badge>}>
        <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
          <MetricTile label="events" value="12,418" hint="last 24h" />
          <MetricTile label="subjects" value="1,204" />
          <MetricTile label="objects" value="4,891" />
          <MetricTile label="dead letter" value="0" hint="catalog backlog" />
        </div>
      </Panel>

      <Panel title="Table">
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
              <Td><StatusToken state="ok" /></Td>
              <Td mono>sku_4291</Td>
              <Td>14:02 today</Td>
              <Td mono align="right">1</Td>
            </Tr>
            <Tr>
              <Td><StatusToken state="run" /></Td>
              <Td mono>sku_4289</Td>
              <Td>14:01 today</Td>
              <Td mono align="right">1</Td>
            </Tr>
            <Tr>
              <Td><StatusToken state="fail" /></Td>
              <Td mono>sku_4042</Td>
              <Td>13:55 today</Td>
              <Td mono align="right">5</Td>
            </Tr>
            <Tr>
              <Td><StatusToken state="warn" /></Td>
              <Td mono>sku_3987</Td>
              <Td>13:51 today</Td>
              <Td mono align="right">2</Td>
            </Tr>
          </Tbody>
        </Table>
      </Panel>

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
        <Panel title="Key value list">
          <KeyValueList
            rows={[
              { label: 'strategy', value: 'item2vec' },
              { label: 'dim', value: '128' },
              { label: 'catalog auto-embed', value: 'enabled' },
              { label: 'catalog backlog', value: '0' },
            ]}
          />
        </Panel>
        <Panel title="Empty state">
          <EmptyState
            title="No items yet"
            description="Catalog ingest hasn't produced any items for this namespace."
            action={<Button variant="secondary">Open catalog config</Button>}
          />
        </Panel>
      </div>

      <Panel title="Loading state">
        <LoadingState rows={4} />
      </Panel>

      <Panel title="Buttons">
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="primary" size="sm">primary · sm</Button>
          <Button variant="primary">primary · md</Button>
          <Button variant="primary" size="lg">primary · lg</Button>
          <Button variant="secondary">secondary</Button>
          <Button variant="ghost">ghost</Button>
          <Button variant="danger">danger</Button>
          <Button variant="primary" loading>loading</Button>
          <Button variant="secondary" disabled>disabled</Button>
        </div>
      </Panel>

      <Panel title="Toolbar + filters">
        <Toolbar>
          <Field label="state" htmlFor="ks-state">
            <Select id="ks-state" selectSize="sm" defaultValue="all">
              <option value="all">all</option>
              <option value="ok">ok</option>
              <option value="fail">fail</option>
            </Select>
          </Field>
          <Field label="object id" htmlFor="ks-objid">
            <Input id="ks-objid" inputSize="sm" placeholder="sku_" />
          </Field>
          <div className="ml-auto flex items-center gap-2">
            <Button variant="secondary" size="sm">Refresh</Button>
            <Button variant="primary" size="sm">Redrive dead-letter</Button>
          </div>
        </Toolbar>
      </Panel>

      <Panel title="Form" actions={<Badge>local</Badge>}>
        <Form>
          <FormGrid columns={2}>
            <Field label="namespace name" htmlFor="ks-name" required>
              <Input
                id="ks-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="prod"
              />
            </Field>
            <Field label="embedding dim" htmlFor="ks-dim" hint="match your strategy output">
              <NumberInput id="ks-dim" defaultValue={128} />
            </Field>
          </FormGrid>
          <FormGrid columns={2}>
            <Field label="dense strategy" htmlFor="ks-strategy">
              <Select id="ks-strategy" defaultValue="item2vec">
                <option value="item2vec">item2vec</option>
                <option value="svd">svd</option>
                <option value="byoe">byoe</option>
                <option value="disabled">disabled</option>
              </Select>
            </Field>
            <Field label="alpha" htmlFor="ks-alpha" hint="0.0 — 1.0">
              <NumberInput id="ks-alpha" defaultValue={0.7} step={0.05} min={0} max={1} />
            </Field>
          </FormGrid>
          <div className="flex justify-end gap-2 mt-1">
            <Button variant="ghost">Cancel</Button>
            <Button variant="primary" type="submit">Save</Button>
          </div>
        </Form>
      </Panel>

      <Panel title="Overlays">
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="secondary" onClick={() => setModalOpen(true)}>
            Open modal
          </Button>
          <Button variant="danger" onClick={() => setConfirmOpen(true)}>
            Open confirm
          </Button>
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
                <DropdownItem onSelect={close} destructive>Delete</DropdownItem>
              </>
            )}
          </Dropdown>
          <span className="text-sm text-muted">
            keyboard hint <Kbd>Cmd+K</Kbd> opens the command palette.
          </span>
        </div>
      </Panel>

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
        <Panel title="Booleans — Checkbox + Switch">
          <div className="flex flex-col gap-3">
            <label className="flex items-center gap-2 cursor-pointer">
              <Checkbox
                checked={bulkAll}
                onChange={(e) => setBulkAll(e.target.checked)}
              />
              <span className="text-sm text-primary">Select all rows</span>
            </label>
            <label className="flex items-center gap-2 cursor-pointer">
              <Checkbox indeterminate />
              <span className="text-sm text-primary">
                Indeterminate (partial selection)
              </span>
            </label>
            <label className="flex items-center gap-2 cursor-pointer opacity-50">
              <Checkbox disabled />
              <span className="text-sm text-primary">Disabled</span>
            </label>

            <div className="border-t border-default pt-3 mt-1 flex flex-col gap-2">
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
          </div>
        </Panel>

        <Panel title="Single-choice — Radio">
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
      </div>

      <Panel title="Pagination">
        <Table>
          <Thead>
            <Tr>
              <Th>
                <Checkbox
                  checked={bulkRow.r1 && bulkRow.r2}
                  indeterminate={
                    (bulkRow.r1 || bulkRow.r2) && !(bulkRow.r1 && bulkRow.r2)
                  }
                  onChange={(e) =>
                    setBulkRow({ r1: e.target.checked, r2: e.target.checked })
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
                  checked={bulkRow.r1}
                  onChange={(e) =>
                    setBulkRow((p) => ({ ...p, r1: e.target.checked }))
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
                  checked={bulkRow.r2}
                  onChange={(e) =>
                    setBulkRow((p) => ({ ...p, r2: e.target.checked }))
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

      <Panel title="CodeBlock" actions={<Badge>json</Badge>}>
        <CodeBlock language="json" copyable maxHeight="14rem">
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
      </Panel>

      <Modal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        title="Modal example"
        footer={
          <>
            <Button variant="ghost" onClick={() => setModalOpen(false)}>Cancel</Button>
            <Button variant="primary" onClick={() => setModalOpen(false)}>OK</Button>
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
