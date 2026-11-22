import * as Dialog from '@radix-ui/react-dialog'
import { useQuery } from '@tanstack/react-query'
import { Activity, Boxes, Building2, ChevronDown, LayoutDashboard, LogOut, Monitor, PanelLeft, Plus, Search, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { Link, NavLink, useNavigate } from 'react-router'
import { request } from '../lib/api'
import { useDebounced } from '../lib/inventory'
import type { AssetPage } from '../lib/inventory'

export function ConsoleShell({ page, user, permissions, signingOut, onSignOut, children }: {
  page: 'dashboard' | 'assets' | 'agents' | 'organization'; user: string; permissions: string[];
  signingOut: boolean; onSignOut: () => void; children: ReactNode
}) {
  const [compact, setCompact] = useState(() => window.localStorage.getItem('cryol.sidebar.compact') === 'true')
  const [searchOpen, setSearchOpen] = useState(false)
  const canRead = permissions.includes('assets.read')
  const canCreate = canRead && permissions.includes('assets.create')
  const canSettings = permissions.includes('settings.read')
  useEffect(() => {
    if (!canRead) return
    function shortcut(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        setSearchOpen(true)
      }
    }
    window.addEventListener('keydown', shortcut)
    return () => window.removeEventListener('keydown', shortcut)
  }, [canRead])
  return (
    <div className={compact ? 'shell shell-compact' : 'shell'}>
      <aside className="shell-sidebar">
        <Link to="/console/dashboard" className="shell-brand" title="Cryol Asset Management">
          <span className="brand-icon"><Boxes size={20} /></span><span className="nav-label">Cryol</span>
        </Link>
        <nav aria-label="Main navigation">
          <NavLink to="/console/dashboard" title="Dashboard"><LayoutDashboard /><span className="nav-label">Dashboard</span></NavLink>
          {canRead && <NavLink to="/console/assets" title="Assets"><Boxes /><span className="nav-label">Assets</span></NavLink>}
          {permissions.includes('agents.read') && <NavLink to="/console/agents" title="Endpoint agents"><Monitor /><span className="nav-label">Endpoint agents</span></NavLink>}
          <details open>
            <summary title="Administration"><Building2 /><span className="nav-label">Administration</span><ChevronDown className="group-chevron" /></summary>
            {canSettings && <NavLink to="/console/settings/organization" title="Organization settings"><Building2 /><span className="nav-label">Organization settings</span></NavLink>}
            <NavLink to="/system" title="System status"><Activity /><span className="nav-label">System status</span></NavLink>
          </details>
        </nav>
      </aside>
      <div className="shell-main">
        <header className="topbar">
          <button className="icon-button" aria-label={compact ? 'Expand sidebar' : 'Collapse sidebar'} onClick={() => {
            window.localStorage.setItem('cryol.sidebar.compact', String(!compact))
            setCompact(!compact)
          }}><PanelLeft size={18} /></button>
          <nav className="top-crumb" aria-label="Breadcrumb">
            <Link to="/console/dashboard">Console</Link>
            <span aria-hidden="true">/</span>
            <span aria-current="page">{{ dashboard: 'Dashboard', assets: 'Assets', agents: 'Endpoint agents', organization: 'Organization settings' }[page]}</span>
          </nav>
          <div className="top-actions">
            {canRead && <SearchPalette open={searchOpen} onOpenChange={setSearchOpen} />}
            {canCreate && page !== 'assets' && <Link className="button" to="/console/assets?mode=create"><Plus size={16} /> Add asset</Link>}
            <details className="account-menu">
              <summary><span className="avatar" aria-hidden="true">{user.slice(0, 2).toUpperCase()}</span><span className="account-name">{user}</span><ChevronDown size={14} /></summary>
              <div className="account-popover">
                {canSettings && <Link to="/console/settings/organization">Organization settings</Link>}
                <button onClick={onSignOut} disabled={signingOut}><LogOut size={15} /> Sign out</button>
              </div>
            </details>
          </div>
        </header>
        <main className="shell-content">{children}</main>
      </div>
    </div>
  )
}

function SearchPalette({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [query, setQuery] = useState('')
  const q = useDebounced(query.trim())
  const navigate = useNavigate()
  const results = useQuery({
    queryKey: ['assets', 'search', q],
    queryFn: ({ signal }) => request<AssetPage>('/assets?' + new URLSearchParams({ q }), 'GET', undefined, signal),
    enabled: open && q.length >= 2, retry: false,
  })
  function change(next: boolean) {
    if (!next) setQuery('')
    onOpenChange(next)
  }
  function go(url: string) {
    change(false)
    void navigate(url)
  }
  return (
    <Dialog.Root open={open} onOpenChange={change}>
      <Dialog.Trigger asChild><button className="top-search" aria-label="Search assets"><Search size={16} /><span>Search assets</span><kbd>Ctrl / ⌘ K</kbd></button></Dialog.Trigger>
      {open && <Dialog.Portal>
        <Dialog.Overlay className="overlay" />
        <Dialog.Content className="search-palette" onKeyDown={(event) => {
          if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
          const targets = [...event.currentTarget.querySelectorAll<HTMLElement>('input, [data-search-result]')]
          if (targets.length < 2) return
          event.preventDefault()
          const index = targets.indexOf(document.activeElement as HTMLElement)
          targets[(index + (event.key === 'ArrowDown' ? 1 : targets.length - 1)) % targets.length]?.focus()
        }}>
          <header><div><Dialog.Title>Search assets</Dialog.Title><Dialog.Description>Find assets by tag, name or serial number.</Dialog.Description></div><Dialog.Close asChild><button className="icon-button" aria-label="Close search"><X size={18} /></button></Dialog.Close></header>
          <form onSubmit={(event) => { event.preventDefault(); go('/console/assets?' + new URLSearchParams({ q: query.trim() })) }}>
            <input aria-label="Asset search query" placeholder="Enter at least two characters" maxLength={120} value={query} onChange={(event) => setQuery(event.target.value)} />
          </form>
          <div className="search-results" aria-live="polite">
            {q.length >= 2 && query.trim() === q && (results.isPending ? <p>Searching…</p> : results.isError ? <p role="alert">{results.error.message}</p> : results.data?.items.length ? <>
              <h3>Assets</h3>
              {results.data.items.slice(0, 8).map((asset) => <button data-search-result key={asset.id} onClick={() => go('/console/assets?' + new URLSearchParams({ q, asset: asset.id }))}><Boxes size={16} /><span>{asset.name}</span><small>{asset.number}</small></button>)}
              <button data-search-result onClick={() => go('/console/assets?' + new URLSearchParams({ q }))}>View all matching assets</button>
            </> : <p>No matching assets.</p>)}
          </div>
        </Dialog.Content>
      </Dialog.Portal>}
    </Dialog.Root>
  )
}
