import { useState, type FormEvent } from 'react'
import { Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { Banner, Button, Card, Stack, Text, TextInput } from '@astryxdesign/core'
import PageContainer from '@/components/PageContainer'
import { useLogin, useSession } from '@/services/auth'

export default function LoginPage() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
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
    login.mutate({ username, password }, {
      onSuccess: () => navigate(next, { replace: true }),
    })
  }

  return (
    <PageContainer size="sm" className="py-16">
      <Card>
        <Stack gap={1}>
          <Text weight="semibold">codohue admin</Text>
          <Text type="supporting">Sign in with your operator account.</Text>
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
                label="Username"
                value={username}
                onChange={setUsername}
              />
              <TextInput
                label="Password"
                value={password}
                onChange={setPassword}
                type="password"
              />
              <Button type="submit" isDisabled={login.isPending || !username || !password} label={login.isPending ? 'Signing in…' : 'Sign in'} />
            </Stack>
          </form>
      </Card>
    </PageContainer>
  )
}
