import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Activity, Boxes, Eye, EyeOff, LoaderCircle } from 'lucide-react'
import { useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useNavigate } from 'react-router'
import { RequestError, request } from './lib/api'
import { ConsoleShell } from './components/console-shell'
import { dashboardKey } from './lib/inventory'
import { Assets } from './asset-registry'
import { Agents } from './agent-console'

type User = { username: string; permissions: string[] }
type Profile = {
  name: string; address: string; contact: string; timezone: string
  currency: string; financialYearMonth: number; assetPrefix: string
}

const accountKey = ['account']
const profileKey = ['organization-profile']

export function Workspace({ page = "organization" }: { page?: "dashboard" | "assets" | "agents" | "organization" }) {
  const client = useQueryClient()
  const account = useQuery({
    queryKey: accountKey,
    queryFn: async () => {
      try { return await request<User>('/auth/me') }
      catch (error) {
        if (error instanceof RequestError && error.status === 401) return null
        throw error
      }
    },
    retry: false,
  })
  const logout = useMutation({
    mutationFn: () => request('/auth/logout', 'POST'),
    onSuccess: () => {
      client.setQueryData(accountKey, null)
      client.removeQueries({ queryKey: profileKey })
      client.removeQueries({ queryKey: ['assets'] })
      client.removeQueries({ queryKey: dashboardKey })
      client.removeQueries({ queryKey: ['agents'] })
    },
  })
  if (account.data) {
    const permissions = account.data.permissions
    return (
      <ConsoleShell page={page} user={account.data.username} permissions={permissions}
        onSignOut={() => logout.mutate()} signingOut={logout.isPending}>
        {logout.isError && <p role="alert">{logout.error.message}</p>}
        {page === 'dashboard' && <Dashboard user={account.data} />}
        {page === 'assets' && (permissions.includes('assets.read')
          ? <Assets permissions={permissions} />
          : <p>You do not have permission to view assets.</p>)}
        {page === 'agents' && (permissions.includes('agents.read')
          ? <Agents permissions={permissions} />
          : <p>You do not have permission to view agents.</p>)}
        {page === 'organization' && (permissions.includes('settings.read')
          ? <Organization canEdit={permissions.includes('settings.manage')} />
          : <p>You do not have permission to view organization settings.</p>)}
      </ConsoleShell>
    )
  }
  return (
    <main className="auth-page">
      <div className="auth-layout">
        <section className="auth-card" aria-label="Cryol account access">
          <div className="auth-brand"><span className="brand-icon"><Boxes size={20} aria-hidden="true" /></span>Cryol Asset Management</div>
          {account.isPending && <p className="auth-status" role="status">Checking your session…</p>}
          {account.isError && <><p className="auth-error" role="alert">{account.error.message}</p><button onClick={() => void account.refetch()}>Retry</button></>}
          {account.data === null && <Login />}
        </section>
        <Link className="auth-system-link" to="/system"><Activity size={15} aria-hidden="true" /> System status</Link>
      </div>
    </main>
  )
}

function Login() {
  const client = useQueryClient()
  const [pending, setPending] = useState(false)
  const [message, setMessage] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const passwordField = useRef<HTMLInputElement>(null)
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return
    setPending(true)
    setMessage('')
    const form = event.currentTarget
    const data = new FormData(form)
    try {
      await request('/auth/login', 'POST', { username: String(data.get('username')), password: String(data.get('password')) })
      await client.invalidateQueries({ queryKey: accountKey })
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Sign-in failed.')
      if (passwordField.current) passwordField.current.value = ''
      setShowPassword(false)
    } finally {
      setPending(false)
    }
  }
  return (
    <>
      <h1 className="auth-title">Sign in</h1>
      <form className="login-form" onSubmit={submit}>
        <div className="auth-field">
          <label htmlFor="cryol-username">Username</label>
          <input id="cryol-username" name="username" autoComplete="username" autoCapitalize="none" spellCheck={false} required maxLength={64} readOnly={pending} />
        </div>
        <div className="auth-field">
          <label htmlFor="cryol-password">Password</label>
          <div className="auth-password">
            <input id="cryol-password" ref={passwordField} name="password" type={showPassword ? 'text' : 'password'} autoComplete="current-password" required maxLength={1024} readOnly={pending} />
            <button className="icon-button" type="button" disabled={pending} onClick={() => setShowPassword(!showPassword)} aria-label={showPassword ? 'Hide password' : 'Show password'} aria-controls="cryol-password">
              {showPassword ? <EyeOff size={18} aria-hidden="true" /> : <Eye size={18} aria-hidden="true" />}
            </button>
          </div>
        </div>
        {message && <p className="auth-error" role="alert">{message}</p>}
        <button className="button-primary auth-submit" type="submit" disabled={pending}>
          {pending && <LoaderCircle className="auth-spinner" size={16} aria-hidden="true" />}
          {pending ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
    </>
  )
}

function Organization({ canEdit }: { canEdit: boolean }) {
  const profile = useQuery({ queryKey: profileKey, queryFn: () => request<Profile>('/organization/profile'), retry: false })
  return (
    <>
      <h1>Organization profile</h1>
      {profile.isPending && <p role="status">Loading profile…</p>}
      {profile.isError && <><p role="alert">{profile.error.message}</p><button onClick={() => void profile.refetch()}>Retry</button></>}
      {profile.data && <ProfileForm initial={profile.data} canEdit={canEdit} />}
    </>
  )
}

function ProfileForm({ initial, canEdit }: { initial: Profile; canEdit: boolean }) {
  const client = useQueryClient()
  const navigate = useNavigate()
  const [profile, setProfile] = useState(initial)
  const save = useMutation({
    mutationFn: (value: Profile) => request<Profile>('/organization/profile', 'PUT', value),
    onSuccess: (value) => {
      client.setQueryData(profileKey, value)
      setProfile(value)
      if (!initial.name) void navigate("/console/dashboard", { replace: true })
    },
  })
  return (
    <form className="record-form" onSubmit={(event) => { event.preventDefault(); save.mutate(profile) }}>
      <fieldset disabled={!canEdit || save.isPending}>
        <label>Organization name<input required maxLength={200} value={profile.name} onChange={(e) => setProfile({ ...profile, name: e.target.value })} /></label>
        <label>Address<textarea maxLength={2000} value={profile.address} onChange={(e) => setProfile({ ...profile, address: e.target.value })} /></label>
        <label>Contact details<input maxLength={500} value={profile.contact} onChange={(e) => setProfile({ ...profile, contact: e.target.value })} /></label>
        <label>Time zone<input required value={profile.timezone} onChange={(e) => setProfile({ ...profile, timezone: e.target.value })} /></label>
        <label>Currency code<input required pattern="[A-Z]{3}" value={profile.currency} onChange={(e) => setProfile({ ...profile, currency: e.target.value })} /></label>
        <label>Financial year start month<input required type="number" min={1} max={12} value={profile.financialYearMonth || ''} onChange={(e) => setProfile({ ...profile, financialYearMonth: Number(e.target.value) })} /></label>
        <label>Asset prefix<input required maxLength={16} value={profile.assetPrefix} onChange={(e) => setProfile({ ...profile, assetPrefix: e.target.value })} /></label>
        {canEdit && <button type="submit">{save.isPending ? 'Saving…' : 'Save profile'}</button>}
      </fieldset>
      {save.isSuccess && <p role="status">Profile saved.</p>}
      {save.isError && <p role="alert">{save.error.message}</p>}
    </form>
  )
}

function Dashboard({ user }: { user: User }) {
  const canReadProfile = user.permissions.includes('settings.read')
  const canReadAssets = user.permissions.includes('assets.read')
  const profile = useQuery({
    queryKey: profileKey, queryFn: () => request<Profile>('/organization/profile'), enabled: canReadProfile, retry: false,
  })
  const summary = useQuery({
    queryKey: dashboardKey, queryFn: () => request<{ totalAssets: number; activeAssets: number; archivedAssets: number }>('/dashboard/summary'), enabled: canReadAssets, retry: false,
  })
  return (
    <>
      <h1>Dashboard</h1>
      {canReadProfile && profile.isPending && <p role="status">Loading organization…</p>}
      {profile.isError && <p role="alert">{profile.error.message}</p>}
      {profile.data?.name && <p>{profile.data.name}</p>}
      {profile.data && !profile.data.name && user.permissions.includes('settings.manage') &&
        <p><Link to="/console/settings/organization">Set up your organization</Link> to start registering assets.</p>}
      {canReadAssets && (
        <section aria-label="Asset summary">
          {summary.isPending && <p role="status">Loading assets…</p>}
          {summary.isError && <p role="alert">{summary.error.message} <button onClick={() => void summary.refetch()}>Retry</button></p>}
          {summary.data && <div className="metric-grid">
            <Link className="asset-summary" to="/console/assets"><h2>Total assets</h2><strong>{summary.data.totalAssets}</strong></Link>
            <Link className="asset-summary" to="/console/assets?state=active"><h2>Active assets</h2><strong>{summary.data.activeAssets}</strong></Link>
            <Link className="asset-summary" to="/console/assets?state=archived"><h2>Archived assets</h2><strong>{summary.data.archivedAssets}</strong></Link>
          </div>}
          <p><Link to="/console/assets">Open asset registry</Link></p>
        </section>
      )}
      {!canReadProfile && !canReadAssets && <p>Your account has no dashboard modules enabled.</p>}
    </>
  )
}
