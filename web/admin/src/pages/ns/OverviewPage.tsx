import { useParams } from 'react-router-dom'
import PageStub from '../_stub'

export default function OverviewPage() {
  const { name } = useParams<{ name: string }>()
  return <PageStub title={`Overview · ${name ?? '?'}`} />
}
