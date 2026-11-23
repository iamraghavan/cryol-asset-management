import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router'
import { Workspace } from './workspace'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

it('signs in, loads the authorized profile, and signs out', async () => {
  let authenticated = false
  const fetch = vi.fn(async (path: string, init?: RequestInit) => {
    switch (path) {
      case '/api/v1/auth/me':
        return authenticated
          ? Response.json({ username: 'operator', permissions: ['settings.read'] })
          : new Response(null, { status: 401 })
      case '/api/v1/auth/login':
        expect(init?.method).toBe('POST')
        if (JSON.parse(String(init?.body)).password === 'wrong password') {
          return Response.json({ error: { message: 'Username or password is incorrect.' } }, { status: 401 })
        }
        authenticated = true
        return Response.json({ status: 'authenticated' })
      case '/api/v1/organization/profile':
        return Response.json({ name: 'Example workshop', address: '', contact: '', timezone: 'UTC', currency: 'USD', financialYearMonth: 1, assetPrefix: 'CRY' })
      case '/api/v1/auth/logout':
        authenticated = false
        return Response.json({ status: 'signed-out' })
      default:
        throw new Error('Unexpected request')
    }
  })
  vi.stubGlobal('fetch', fetch)
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><Workspace /></MemoryRouter></QueryClientProvider>)
  await screen.findByRole('heading', { name: 'Sign in' })
  fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'operator' } })
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'wrong password' } })
  fireEvent.click(screen.getByRole('button', { name: 'Show password' }))
  expect(screen.getByLabelText('Password').getAttribute('type')).toBe('text')
  fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))
  expect((await screen.findByRole('alert')).textContent).toBe('Username or password is incorrect.')
  expect((screen.getByLabelText<HTMLInputElement>('Username')).value).toBe('operator')
  expect((screen.getByLabelText<HTMLInputElement>('Password')).value).toBe('')
  expect(screen.getByLabelText('Password').getAttribute('type')).toBe('password')
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'temporary password' } })
  fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))
  await screen.findByDisplayValue('Example workshop')
  expect(screen.queryByRole('button', { name: 'Save profile' })).toBeNull()
  fireEvent.click(screen.getByText('operator'))
  fireEvent.click(screen.getByRole('button', { name: 'Sign out' }))
  await screen.findByRole('heading', { name: 'Sign in' })
  expect(screen.queryByDisplayValue('Example workshop')).toBeNull()
})

it('takes a newly configured organization to the dashboard', async () => {
  let profile = { name: '', address: '', contact: '', timezone: 'UTC', currency: 'USD', financialYearMonth: 1, assetPrefix: 'CRY' }
  vi.stubGlobal('fetch', vi.fn(async (path: string, init?: RequestInit) => {
    if (path === '/api/v1/auth/me') {
      return Response.json({ username: 'admin', permissions: ['settings.read', 'settings.manage', 'assets.read'] })
    }
    if (path === '/api/v1/organization/profile') {
      if (init?.method === 'PUT') profile = JSON.parse(String(init.body))
      return Response.json(profile)
    }
    if (path === '/api/v1/dashboard/summary') return Response.json({ totalAssets: 0 })
    throw new Error('Unexpected request')
  }))
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/console/settings/organization']}>
        <Routes>
          <Route path="/console/settings/organization" element={<Workspace />} />
          <Route path="/console/dashboard" element={<Workspace page="dashboard" />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
  fireEvent.change(await screen.findByLabelText('Organization name'), { target: { value: 'Example workshop' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save profile' }))
  await screen.findByRole('heading', { name: 'Dashboard' })
  await screen.findByText('Example workshop')
  expect(screen.getByRole('link', { name: 'Open asset registry' }).getAttribute('href')).toBe('/console/assets')
})
