import { useState } from 'react'
import {
  Badge,
  Button,
  CodeBadge,
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
  Panel,
  Select,
  StatusToken,
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
                Actions ▾
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
            keyboard hint <Kbd>⌘K</Kbd> opens the command palette.
          </span>
        </div>
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
