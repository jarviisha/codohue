import { useState, type FormEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { ApiError } from '@/services/http'
import { useLogin } from '@/services/auth'
import { Button, Field, Form, Input, Notice } from '@/components/ui'

export default function LoginPage() {
  const [apiKey, setApiKey] = useState('')
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const next = params.get('next') || '/'
  const login = useLogin()

  const submit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (!apiKey.trim() || login.isPending) return
    login.mutate(
      { api_key: apiKey },
      {
        onSuccess: () => {
          navigate(next, { replace: true })
        },
      },
    )
  }

  return (
    <main className="min-h-screen bg-base text-primary flex items-center justify-center px-6">
      <div className="w-90 border border-default rounded-sm bg-surface p-6">
        <div className="font-mono text-[11px] font-semibold uppercase tracking-[0.12em] text-muted mb-1">
          codohue
        </div>
        <h1 className="text-xl font-semibold text-primary leading-tight mb-4">
          Sign in
        </h1>

        {login.isError ? (
          <div className="mb-3">
            <Notice tone="fail">{describeLoginError(login.error)}</Notice>
          </div>
        ) : null}

        <Form onSubmit={submit}>
          <Field label="Admin API key" htmlFor="api-key" required>
            <Input
              id="api-key"
              type="password"
              autoFocus
              autoComplete="current-password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder="RECOMMENDER_API_KEY"
            />
          </Field>
          <Button
            type="submit"
            variant="primary"
            size="lg"
            loading={login.isPending}
            disabled={!apiKey.trim()}
          >
            Sign in
          </Button>
        </Form>
      </div>
    </main>
  )
}

function describeLoginError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 401) return 'Invalid API key.'
    return err.message
  }
  if (err instanceof Error) return err.message
  return 'Sign-in failed.'
}
