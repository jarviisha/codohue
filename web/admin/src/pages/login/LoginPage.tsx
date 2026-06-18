import { useState, type FormEvent } from 'react'
import { Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import {
  Alert,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Container,
  FormField,
  Input,
  Stack,
} from '@jarviisha/davinci-react-ui'
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
    <Container size="sm" className="py-16">
      <Card>
        <CardHeader>
          <CardTitle>codohue admin</CardTitle>
          <CardDescription>Sign in with the global admin API key.</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit}>
            <Stack>
              {login.error && (
                <Alert
                  variant="danger"
                  title="Sign-in failed"
                  description={login.error.message}
                />
              )}
              <FormField label="API key" required>
                <Input
                  type="password"
                  value={apiKey}
                  onChange={(e) => setApiKey(e.target.value)}
                  autoFocus
                  autoComplete="current-password"
                  required
                />
              </FormField>
              <Button type="submit" disabled={login.isPending || apiKey.length === 0}>
                {login.isPending ? 'Signing in…' : 'Sign in'}
              </Button>
            </Stack>
          </form>
        </CardContent>
      </Card>
    </Container>
  )
}
