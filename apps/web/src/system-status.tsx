import { useQuery } from '@tanstack/react-query'

async function readReadiness(signal: AbortSignal) {
  const response = await fetch('/api/v1/system/readiness', { signal })
  if (response.status === 503) return 'Database unavailable'
  if (!response.ok) throw new Error('Readiness request failed')
  const payload: unknown = await response.json()
  if (!payload || typeof payload !== 'object' || !('status' in payload) || payload.status !== 'ready') {
    throw new Error('Invalid readiness response')
  }
  return 'Connected'
}

export function SystemStatus() {
  const readiness = useQuery({
    queryKey: ['system', 'readiness'],
    queryFn: ({ signal }) => readReadiness(signal),
    retry: false,
    refetchInterval: 15_000,
  })

  return (
    <>
      <header className="brand">Cryol Asset Management</header>
      <main>
        <h1>System status</h1>
        <dl className="status">
          <dt>PostgreSQL</dt>
          <dd role="status">
            {readiness.isPending ? 'Checking connection…' : readiness.isError ? 'Could not check readiness' : readiness.data}
          </dd>
        </dl>
        <button disabled={readiness.isFetching} onClick={() => void readiness.refetch()}>
          Refresh status
        </button>
      </main>
    </>
  )
}
