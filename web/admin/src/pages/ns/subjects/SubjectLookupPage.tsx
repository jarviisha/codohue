import { useState, type FormEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import {
  Button,
  Card,
  CardContent,
  Container,
  FormField,
  Inline,
  Input,
  Stack,
} from '@jarviisha/davinci-react-ui'
import PageHeader from '@/components/shell/PageHeader'

/**
 * SubjectLookupPage is the landing page for /ns/:ns/subjects. The admin
 * backend doesn't expose a "list all subjects" endpoint (events table can be
 * huge; subjects aren't an enumerable resource here), so this page is just a
 * direct-input lookup form. Operators paste in the subject id they're
 * debugging and jump to /ns/:ns/subjects/:id.
 */
export default function SubjectLookupPage() {
  const { ns } = useParams<{ ns: string }>()
  const navigate = useNavigate()
  const [id, setId] = useState('')

  if (!ns) return null

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    const trimmed = id.trim()
    if (!trimmed) return
    navigate(`/ns/${encodeURIComponent(ns)}/subjects/${encodeURIComponent(trimmed)}`)
  }

  return (
    <Container size="md" className="py-6 px-6">
      <PageHeader>
        <Stack gap="050">
          <h1 className="text-foreground text-xl font-semibold">Subjects</h1>
          <p className="text-foreground-subtle text-sm">
            Inspect one subject's profile and recommendations. Paste a subject id below to open it.
          </p>
        </Stack>
      </PageHeader>

      <Card>
        <CardContent>
          <form onSubmit={onSubmit}>
            <Stack>
              <FormField
                label="Subject ID"
                required
                helpText="The same id you'd send in /v1/events.subject_id."
              >
                <Input
                  value={id}
                  onChange={(e) => setId(e.target.value)}
                  placeholder="e.g. user-42"
                  autoFocus
                />
              </FormField>
              <Inline justify="end">
                <Button type="submit" disabled={id.trim() === ''}>
                  Open subject
                </Button>
              </Inline>
            </Stack>
          </form>
        </CardContent>
      </Card>
    </Container>
  )
}
