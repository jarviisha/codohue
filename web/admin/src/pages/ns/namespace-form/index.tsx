import { useState, type FormEvent } from 'react'
import { Form, Notice } from '@/components/ui'
import {
  hasErrors,
  normalizeNamespaceName,
  validateNamespaceForm,
  type ActionWeightRow,
  type NamespaceFormErrors,
  type NamespaceFormState,
} from '../configForm'
import ActionsTab from './ActionsTab'
import DenseTab from './DenseTab'
import IdentityTab from './IdentityTab'
import ScoringTab from './ScoringTab'
import TabNav from './TabNav'
import TrendingTab from './TrendingTab'
import { firstErrorTab, tabsForMode } from './tabs'
import type { TabId } from './types'

interface NamespaceFormProps {
  mode: 'create' | 'edit'
  formId: string
  state: NamespaceFormState
  onChange: (next: NamespaceFormState) => void
  onSubmit: (values: NamespaceFormState) => void
  isPending: boolean
  errorMessage?: string
}

export default function NamespaceForm({
  mode,
  formId,
  state,
  onChange,
  onSubmit,
  isPending,
  errorMessage,
}: NamespaceFormProps) {
  const tabs = tabsForMode(mode)
  const [activeTab, setActiveTab] = useState<TabId>(tabs[0])
  const [errors, setErrors] = useState<NamespaceFormErrors>(() => ({}))
  const [submitted, setSubmitted] = useState(false)

  const propagate = (next: NamespaceFormState) => {
    onChange(next)
    if (submitted) setErrors(validateNamespaceForm(next, mode))
  }

  const update = <K extends keyof NamespaceFormState>(
    key: K,
    value: NamespaceFormState[K],
  ) => {
    propagate({ ...state, [key]: value })
  }

  const updateNumber = <K extends keyof NamespaceFormState>(
    key: K,
    raw: string,
  ) => {
    if (raw.trim() === '') return
    const next = Number(raw)
    if (!Number.isFinite(next)) return
    update(key, next as NamespaceFormState[K])
  }

  const setActions = (rows: ActionWeightRow[]) =>
    propagate({ ...state, action_weights: rows })

  const submit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (isPending) return
    setSubmitted(true)
    const nextState =
      mode === 'create'
        ? { ...state, namespace: normalizeNamespaceName(state.namespace) }
        : state
    if (nextState !== state) onChange(nextState)
    const nextErrors = validateNamespaceForm(nextState, mode)
    setErrors(nextErrors)
    const nextTab = firstErrorTab(nextErrors, mode)
    if (nextTab) setActiveTab(nextTab)
    if (hasErrors(nextErrors)) return
    onSubmit(nextState)
  }

  const tabProps = { state, errors, update, updateNumber }

  return (
    <Form id={formId} onSubmit={submit}>
      {errorMessage ? (
        <Notice tone="fail" title="Save failed">{errorMessage}</Notice>
      ) : null}

      <div className="grid grid-cols-1 lg:grid-cols-[12rem_minmax(0,1fr)] gap-5 items-start">
        <TabNav
          tabs={tabs}
          active={activeTab}
          errors={errors}
          onSelect={setActiveTab}
        />

        <div id={`ns-config-${activeTab}`} role="tabpanel" className="min-w-0">
          {activeTab === 'identity' ? <IdentityTab {...tabProps} /> : null}
          {activeTab === 'actions' ? (
            <ActionsTab {...tabProps} setActions={setActions} />
          ) : null}
          {activeTab === 'scoring' ? <ScoringTab {...tabProps} /> : null}
          {activeTab === 'dense' ? <DenseTab {...tabProps} /> : null}
          {activeTab === 'trending' ? <TrendingTab {...tabProps} /> : null}
        </div>
      </div>
    </Form>
  )
}
