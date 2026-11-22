import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { createBrowserRouter, Link, Navigate, RouterProvider } from 'react-router'
import '@fontsource/inter/latin-400.css'
import '@fontsource/inter/latin-600.css'
import { SystemStatus } from './system-status'
import { Workspace } from './workspace'
import './styles.css'

const root = document.getElementById('root')
if (!root) throw new Error('Missing application root')

const router = createBrowserRouter([
  { path: '/', element: <Navigate to="/console/dashboard" replace /> },
  {
    path: '/console',
    children: [
      { index: true, element: <Navigate to="dashboard" replace /> },
      { path: 'dashboard', element: <Workspace page="dashboard" /> },
      { path: 'assets', element: <Workspace page="assets" /> },
      { path: 'agents', element: <Workspace page="agents" /> },
      { path: 'settings', children: [
        { index: true, element: <Navigate to="organization" replace /> },
        { path: 'organization', element: <Workspace /> },
      ] },
    ],
  },
  { path: '/system', element: <SystemStatus /> },
  { path: '*', element: <main><h1>Page unavailable</h1><Link to="/">Home</Link></main> },
])

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={new QueryClient()}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
)
