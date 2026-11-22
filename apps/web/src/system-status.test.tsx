import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { SystemStatus } from './system-status'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

it('reads live readiness and refreshes a degraded connection', async () => {
  const fetch = vi.fn()
    .mockResolvedValueOnce(new Response(null, { status: 503 }))
    .mockResolvedValueOnce(Response.json({ status: 'ready' }))
  vi.stubGlobal('fetch', fetch)

  render(<QueryClientProvider client={new QueryClient()}><SystemStatus /></QueryClientProvider>)

  expect(screen.getByRole('status').textContent).toBe('Checking connection…')
  await screen.findByText('Database unavailable')
  fireEvent.click(screen.getByRole('button', { name: 'Refresh status' }))
  await screen.findByText('Connected')
  expect(fetch).toHaveBeenCalledTimes(2)
  expect(fetch.mock.calls[0]?.[0]).toBe('/api/v1/system/readiness')
})
