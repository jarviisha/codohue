import { useState, type FormEvent } from 'react'
import { Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { Banner, Button, Card, Stack, Text, TextInput } from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'
import { useLogin, useSession } from '@/services/auth'

export default function LoginPage() {
  const [apiKey, setApiKey] = useState('')
  const [searchParams] = useSearchParams()
  const next = searchParams.get('next') ?? '/'
  const navigate = useNavigate()

  const session = useSession()
  const login = useLogin()

  // Already logged in → bounce to the redirect target.
  if (session.isSuccess) {
    return <Navigate to={next} replace />
  }

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    login.mutate(apiKey, {
      onSuccess: () => navigate(next, { replace: true }),
    })
  }

  return (
    <PageContainer size="sm" className="py-16">
      <Card>
        <Stack gap={1}>
          <Text weight="semibold">codohue admin</Text>
          <Text type="supporting">Sign in with the global admin API key.</Text>
        </Stack>
          <form onSubmit={onSubmit}>
            <Stack gap={6}>
              {login.error && (
                <Banner
                  status="error"
                  title="Sign-in failed"
                  description={login.error.message}
                />
              )}
              <TextInput
                label="API key"
                value={apiKey}
                onChange={setApiKey}
                type="password"
              />
              <Button type="submit" isDisabled={login.isPending || apiKey.length === 0} label={login.isPending ? 'Signing in…' : 'Sign in'} />
            </Stack>
          </form>
      </Card>
    </PageContainer>
  )
}
