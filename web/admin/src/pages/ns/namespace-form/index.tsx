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
import ActionsSection from './ActionsSection'
import DenseSection from './DenseSection'
import IdentitySection from './IdentitySection'
import ScoringSection from './ScoringSection'
import TrendingSection from './TrendingSection'
import type { FormContext } from './types'

interface NamespaceFormProps {
  mode: 'create' | 'edit'
  formId: string
  state: NamespaceFormState
  onChange: (next: NamespaceFormState) => void
  onSubmit: (values: NamespaceFormState) => void
  isPending: boolean
  errorMessage?: string
  context?: FormContext
}

const DEFAULT_CONTEXT: FormContext = { catalogEnabled: false }

// DOM id of the input (or the action-weights row container) to focus when a
// field fails validation on submit. The order matters — submit() walks this
// list and jumps to the first matching error.
const ERROR_TARGET_ID: Record<keyof NamespaceFormErrors, string> = {
  namespace: 'ns-name',
  action_weights: 'ns-section-actions',
  lambda: 'ns-lambda',
  gamma: 'ns-gamma',
  alpha: 'ns-alpha',
  max_results: 'ns-maxr',
  seen_items_days: 'ns-seen',
  embedding_dim: 'ns-dim',
  trending_window: 'ns-tr-win',
  trending_ttl: 'ns-tr-ttl',
  lambda_trending: 'ns-tr-lambda',
}

const ERROR_ORDER_CREATE: (keyof NamespaceFormErrors)[] = [
  'namespace',
  'action_weights',
  'lambda',
  'gamma',
  'alpha',
  'max_results',
  'seen_items_days',
  'embedding_dim',
  'trending_window',
  'trending_ttl',
  'lambda_trending',
]
const ERROR_ORDER_EDIT = ERROR_ORDER_CREATE.filter((k) => k !== 'namespace')

function firstErrorTargetId(
  errors: NamespaceFormErrors,
  mode: 'create' | 'edit',
): string | null {
  const order = mode === 'create' ? ERROR_ORDER_CREATE : ERROR_ORDER_EDIT
  for (const key of order) {
    if (errors[key]) return ERROR_TARGET_ID[key]
  }
  return null
}

function scrollToError(id: string) {
  const el = document.getElementById(id)
  if (!el) return
  el.scrollIntoView({ behavior: 'smooth', block: 'center' })
  if (typeof (el as HTMLElement).focus === 'function') {
    ;(el as HTMLElement).focus({ preventScroll: true })
  }
}

export default function NamespaceForm({
  mode,
  formId,
  state,
  onChange,
  onSubmit,
  isPending,
  errorMessage,
  context = DEFAULT_CONTEXT,
}: NamespaceFormProps) {
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
    if (hasErrors(nextErrors)) {
      const target = firstErrorTargetId(nextErrors, mode)
      if (target) scrollToError(target)
      return
    }
    onSubmit(nextState)
  }

  const sectionProps = { state, errors, context, update, updateNumber }

  return (
    <Form id={formId} onSubmit={submit}>
      {errorMessage ? (
        <Notice tone="fail" title="Save failed">{errorMessage}</Notice>
      ) : null}

      {mode === 'create' ? <IdentitySection {...sectionProps} /> : null}
      <div id="ns-section-actions">
        <ActionsSection {...sectionProps} setActions={setActions} />
      </div>
      <ScoringSection {...sectionProps} />
      <DenseSection {...sectionProps} />
      <TrendingSection {...sectionProps} />
    </Form>
  )
}
