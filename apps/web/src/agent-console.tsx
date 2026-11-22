import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, Monitor, Plus, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { RecordDrawer } from './components/record-drawer'
import { useDebounced } from './lib/inventory'
import type { AssetPage } from './lib/inventory'
import { detectAgentPlatform } from './lib/agent-platform'
import type { AgentPlatform } from './lib/agent-platform'
import { request, RequestError } from './lib/api'

type Agent = {
  id: string; deviceId: string; hostname: string; osFamily: string; architecture: string
  version: string; lastSeen: string; lastInventoryAt: string | null; status: string; assetId: string; assetNumber: string
}
type Enrollment = { id: string; token: string; expiresAt: string }
type Hardware = {
  osName: string; logicalCpus: number; cpuModel?: string; memoryBytes?: number; manufacturer?: string; model?: string
  serial?: string; scannedAt: string; softwareStatus: string
  firmwareVendor?: string; firmwareVersion?: string
  volumes?: { mount: string; capacityBytes: number; freeBytes: number }[]
  network?: { name: string; up: boolean; mac?: string; addresses?: string[] }[]
}
type Software = { id: string; source: string; name: string; version: string; publisher?: string }

const date = (value: string | null) => value ? new Date(value).toLocaleString() : '—'

export function Agents({ permissions }: { permissions: string[] }) {
  const [selected, setSelected] = useState<string | null>(null)
  const [enrolling, setEnrolling] = useState(false)
  const [page, setPage] = useState(1)
  const list = useQuery({
    queryKey: ['agents', 'list', page],
    queryFn: ({ signal }) => request<{ items: Agent[]; hasNext: boolean }>('/agents?page=' + page, 'GET', undefined, signal),
    refetchInterval: 60_000, retry: false,
  })
  const agent = list.data?.items.find((item) => item.id === selected)
  return <>
    <header className="page-heading">
      <div><h1>Endpoint agents</h1><p>Review enrolled devices and their latest reported inventory.</p></div>
      {permissions.includes('agents.enroll') && <button className="button-primary" onClick={() => setEnrolling(true)}><Plus size={17} /> New enrollment</button>}
    </header>
    <section className="table-panel" aria-label="Registered agents">
      <div className="table-toolbar"><button onClick={() => void list.refetch()} disabled={list.isFetching}><RefreshCw size={15} /> Refresh</button></div>
      {list.isPending && <p className="table-state" role="status">Loading agents…</p>}
      {list.isError && <p className="table-state" role="alert">{list.error.message}</p>}
      {list.data?.items.length === 0 && <div className="table-state"><Monitor size={24} /><h2>No agents enrolled</h2></div>}
      {!!list.data?.items.length && <div className="asset-table-wrap"><table className="asset-table">
        <thead><tr><th scope="col">Device</th><th scope="col">Platform</th><th scope="col">Agent version</th><th scope="col">Status</th><th scope="col">Last contact</th><th scope="col">Last scan</th></tr></thead>
        <tbody>{list.data.items.map((item) => <tr key={item.id}>
          <td><button className="text-button" onClick={() => setSelected(item.id)}>{item.hostname || 'Awaiting first inventory'}</button></td>
          <td>{item.osFamily || '—'} {item.architecture}</td><td>{item.version || '—'}</td>
          <td><span className={item.status === 'online' ? 'badge' : 'badge badge-muted'}>{item.status}</span></td>
          <td>{date(item.lastSeen)}</td><td>{date(item.lastInventoryAt)}</td>
        </tr>)}</tbody>
      </table></div>}
      {(page > 1 || list.data?.hasNext) && <div className="table-pagination">
        <button disabled={page === 1} onClick={() => setPage(page - 1)}>Previous</button>
        <span>Page {page}</span>
        <button disabled={!list.data?.hasNext} onClick={() => setPage(page + 1)}>Next</button>
      </div>}
    </section>
    {enrolling && <Enroll close={() => setEnrolling(false)} />}
    {agent && <Details item={agent} permissions={permissions} close={() => setSelected(null)} />}
  </>
}

const platformLabels: Record<AgentPlatform, string> = {
  windows: 'Windows', linux: 'Linux', macos: 'macOS', android: 'Android', ios: 'iPhone / iPad', other: 'Other / unknown',
}

function Enroll({ close }: { close: () => void }) {
  const [detected] = useState(() => detectAgentPlatform(navigator.userAgent, navigator.platform, navigator.maxTouchPoints))
  const [platform, setPlatform] = useState<AgentPlatform>(detected)
  const [credential, setCredential] = useState<Enrollment>()
  const [copied, setCopied] = useState(false)
  const setup = useQuery({
    queryKey: ['agents', 'setup'],
    queryFn: ({ signal }) => request<{ origin: string; platforms: string[]; windowsInstaller: boolean; windowsInstallerTest: boolean; windowsInstallerBundledCA: boolean; publicCA: boolean }>('/agents/setup', 'GET', undefined, signal),
    retry: false,
  })
  const issue = useMutation({
    gcTime: 0, mutationFn: () => request<Enrollment>('/agents/enrollment-tokens', 'POST'),
    onSuccess: (value) => { setCredential(value); issue.reset() },
  })
  const revoke = useMutation({
    mutationFn: () => request('/agents/enrollment-tokens/' + credential?.id + '/revoke', 'POST'),
    onSuccess: close,
  })
  return <RecordDrawer title="Enroll an endpoint" description="Create a single-use token for a device you manage." onClose={close}>
    <div className="drawer-body">
      {setup.isPending && <p role="status">Checking setup downloads…</p>}
      {setup.isError && <p role="alert">{setup.error.message} <button onClick={() => void setup.refetch()}>Retry</button></p>}
      {setup.data && <div className="field-grid">
        <label className="field-wide">Endpoint operating system
          <select value={platform} onChange={(event) => setPlatform(event.target.value as AgentPlatform)}>
            {Object.entries(platformLabels).map(([value, label]) => <option key={value} value={value}>
              {label}{value === detected && value !== 'other' ? ' (this device)' : ''}
            </option>)}
          </select>
        </label>
      </div>}
      {setup.data?.origin && <p><strong>Server</strong> {setup.data.origin}</p>}
      {setup.data?.origin.startsWith('http:') && <div className="confirmation">
        <p>This download is for a test agent on this computer.</p>
        <p>For a separate endpoint, open this console using the Cryol server’s LAN HTTPS address before downloading.</p>
      </div>}
      {setup.data && (platform === 'android' || platform === 'ios' || platform === 'other') &&
        <p role="status">A Cryol agent is unavailable for {platformLabels[platform] === 'Other / unknown' ? 'this platform' : platformLabels[platform]}. Choose the operating system of another managed endpoint if you are downloading for it.</p>}
      {setup.data && platform === 'windows' && (setup.data.windowsInstaller ? <section aria-label="Windows setup">
        <h3>Windows x64</h3>
        <div className="row-actions">
          <a className="button button-primary" href="/api/v1/agents/setup/windows-installer"><Download size={16} /> Download Windows x64 installer</a>
          {setup.data.publicCA && !setup.data.windowsInstallerBundledCA && <a className="button" href="/api/v1/agents/setup/ca"><Download size={16} /> Public CA certificate</a>}
        </div>
        {setup.data.windowsInstallerTest && <p role="note"><strong>Unsigned test build.</strong> Use it on a disposable Windows test machine.</p>}
        {setup.data.windowsInstallerBundledCA
          ? <p>The installer includes Cryol’s public CA. Setup uses it automatically.</p>
          : setup.data.publicCA && <p>For this private HTTPS server, select the public CA certificate in the wizard.</p>}
        <ol>
          <li>Run <strong>Cryol-Agent-Setup-x64.exe</strong> and approve the Windows administrator prompt.</li>
          <li>Check the server address and run <strong>Test connection</strong>. Then paste a fresh enrollment token.</li>
          <li>Check <strong>Last scan</strong> after setup. The Windows service starts automatically at boot.</li>
        </ol>
      </section> : setup.data.platforms.includes('windows') ? <section aria-label="Windows setup">
        <h3>Windows x64</h3>
        <a className="button" href="/api/v1/agents/setup/windows"><Download size={16} /> Download Windows x64 ZIP</a>
        <p>Extract the ZIP and open <strong>Start Cryol Agent.cmd</strong>. Approve the administrator prompt and paste a fresh token.</p>
      </section> : <p>Windows setup is unavailable on this server.</p>)}
      {setup.data && (platform === 'linux' || platform === 'macos') && <UnixSetup
        platform={platform} available={setup.data.platforms} />}

      {credential && <>
        <p>The token expires at {date(credential.expiresAt)}.</p>
        <label>Enrollment token<input readOnly value={credential.token} spellCheck={false} onFocus={(event) => event.target.select()} /></label>
        {navigator.clipboard && <button onClick={() => {
          void navigator.clipboard.writeText(credential.token).then(() => setCopied(true), () => setCopied(false))
        }}>{copied ? 'Token copied' : 'Copy token'}</button>}
        {revoke.isError && <p role="alert">{revoke.error.message}</p>}
      </>}
      {issue.isError && <p role="alert">{issue.error.message}</p>}
    </div>
    <footer className="drawer-footer">
      <button onClick={close}>Close</button>
      {credential
        ? <button className="danger-button" disabled={revoke.isPending} onClick={() => revoke.mutate()}>Revoke unused token</button>
        : <button className="button-primary" disabled={issue.isPending || ['android', 'ios', 'other'].includes(platform)} onClick={() => issue.mutate()}>Generate token</button>}
    </footer>
  </RecordDrawer>
}

function UnixSetup({ platform, available }: { platform: 'linux' | 'macos'; available: string[] }) {
  const packages = platform === 'macos'
    ? [{ id: 'macos-arm64', label: 'macOS Apple silicon' }, { id: 'macos-intel', label: 'macOS Intel' }]
    : [{ id: 'linux', label: 'Linux x64' }, { id: 'linux-arm64', label: 'Linux ARM64' }]
  const choices = packages.filter((item) => available.includes(item.id))
  if (!choices.length) return <p>{platformLabels[platform]} setup is unavailable on this server.</p>
  return <section aria-label={platformLabels[platform] + ' setup'}>
    <h3>{platformLabels[platform]} setup</h3>
    <div className="row-actions">
      {choices.map((item) => <a key={item.id} className="button" href={'/api/v1/agents/setup/' + item.id}><Download size={16} /> Download {item.label} ZIP</a>)}
    </div>
    <p>Extract the ZIP, run <code>sh start-cryol-agent.sh</code>, and paste a fresh token. Keep the terminal open for updates.</p>
    {platform === 'macos' && <p>The macOS agent currently reports hardware, operating system and network inventory.</p>}
  </section>
}

function Details({ item, permissions, close }: { item: Agent; permissions: string[]; close: () => void }) {
  const [confirm, setConfirm] = useState(false)
  const [softwareTab, setSoftwareTab] = useState(!permissions.includes('hardware.read'))
  const [filter, setFilter] = useState('')
  const q = useDebounced(filter.trim())
  const [page, setPage] = useState(1)
  const client = useQueryClient()
  const base = '/agents/' + item.id
  const hardware = useQuery({
    queryKey: ['agents', item.id, 'hardware'],
    queryFn: ({ signal }) => request<Hardware>(base + '/inventory?part=hardware', 'GET', undefined, signal),
    enabled: !softwareTab && permissions.includes('hardware.read'), retry: false,
  })
  const software = useQuery({
    queryKey: ['agents', item.id, 'software', q, page],
    queryFn: ({ signal }) => request<{ software: Software[]; scannedAt: string; hasNext: boolean }>(base + '/inventory?' + new URLSearchParams({ part: 'software', q, page: String(page) }), 'GET', undefined, signal),
    enabled: softwareTab && permissions.includes('software.read'), retry: false,
  })
  const revoke = useMutation({
    mutationFn: () => request(base + '/revoke', 'POST'),
    onSuccess: async () => { setConfirm(false); await client.invalidateQueries({ queryKey: ['agents', 'list'] }) },
  })
  const info = hardware.data
  const products = software.data?.software
  const error = softwareTab ? software.error : hardware.error
  return <RecordDrawer title={item.hostname || 'Endpoint agent'} description={item.id} onClose={close}>
    <div className="drawer-body">
      <p><strong>Status</strong> {item.status} · <strong>Last contact</strong> {date(item.lastSeen)}</p>
      {permissions.includes('assets.read') && <p><strong>Linked asset</strong> {item.assetNumber || 'None'}</p>}
      {permissions.includes('assets.read') && permissions.includes('devices.manage') && <AssetLink item={item} />}
      <div className="row-actions" aria-label="Inventory sections">
        {permissions.includes('hardware.read') && <button aria-pressed={!softwareTab} onClick={() => setSoftwareTab(false)}>Hardware and network</button>}
        {permissions.includes('software.read') && <button aria-pressed={softwareTab} onClick={() => setSoftwareTab(true)}>Installed software</button>}
      </div>
      {error && <p role="status">{error instanceof RequestError && error.status === 404 ? 'No scan is available for this section.' : error.message}</p>}
      {!softwareTab && hardware.isFetching && <p role="status">Loading hardware…</p>}
      {!softwareTab && info && <>
        <h3>Latest scan</h3><p>{date(info.scannedAt)}</p>
        <dl className="asset-details">
          {Object.entries({ 'Operating system': info.osName || item.osFamily, CPU: info.cpuModel, 'Logical CPUs': info.logicalCpus,
            Memory: info.memoryBytes ? (info.memoryBytes / 2 ** 30).toFixed(1) + ' GiB' : undefined,
            Manufacturer: info.manufacturer, Model: info.model, Serial: info.serial, 'Firmware vendor': info.firmwareVendor, 'Firmware version': info.firmwareVersion, 'Software collector': info.softwareStatus }).map(([key,value]) =>
              <div key={key}><dt>{key}</dt><dd>{value || 'Unknown'}</dd></div>)}
        </dl>
        {!!info.volumes?.length && <>
          <h3>Fixed drive volumes</h3>
          <dl className="asset-details">{info.volumes.map((volume) => <div key={volume.mount}>
            <dt>{volume.mount}</dt><dd>{(volume.freeBytes / 2 ** 30).toFixed(1)} GiB free of {(volume.capacityBytes / 2 ** 30).toFixed(1)} GiB</dd>
          </div>)}</dl>
        </>}
        <h3>Network adapters</h3>
        {info.network?.length ? <dl className="asset-details">{info.network.map((adapter) =>
          <div key={adapter.name}><dt>{adapter.name}</dt><dd>{adapter.up ? 'Up' : 'Down'}{adapter.mac && ' · ' + adapter.mac}{adapter.addresses?.length ? <><br />{adapter.addresses.join(', ')}</> : null}</dd></div>)}</dl> : <p>No network adapters reported.</p>}
      </>}
      {softwareTab && software.isFetching && <p role="status">Loading software…</p>}
      {softwareTab && permissions.includes('software.read') && <>
        <h3>Installed software</h3>
        <label>Filter software<input maxLength={120} value={filter} onChange={(event) => { setFilter(event.target.value); setPage(1) }} /></label>
      </>}
      {softwareTab && products && <>
        <p>Source scan {date(software.data?.scannedAt ?? null)}</p>
        {products.length === 0 && <p>No matching packages.</p>}
        <div className="asset-table-wrap"><table className="asset-table"><thead><tr><th>Application</th><th>Version</th><th>Source</th></tr></thead>
          <tbody>{products.map((product) => <tr key={product.source + product.id}><td>{product.name}{product.publisher && <small> · {product.publisher}</small>}</td><td>{product.version || '—'}</td><td>{product.source}</td></tr>)}</tbody></table></div>
        {(page > 1 || software.data?.hasNext) && <div className="table-pagination">
          <button disabled={page === 1} onClick={() => setPage(page - 1)}>Previous</button><span>Page {page}</span>
          <button disabled={!software.data?.hasNext} onClick={() => setPage(page + 1)}>Next</button>
        </div>}
      </>}
      {confirm && <div className="confirmation">
        <strong>Revoke this agent?</strong><p>The agent will stop submitting inventory. Existing inventory stays available.</p>
        <button disabled={revoke.isPending} onClick={() => setConfirm(false)}>Cancel</button>
        <button className="danger-button" disabled={revoke.isPending} onClick={() => revoke.mutate()}>Revoke agent</button>
        {revoke.isError && <p role="alert">{revoke.error.message}</p>}
      </div>}
    </div>
    <footer className="drawer-footer">{permissions.includes('agents.revoke') && item.status !== 'revoked' && !confirm &&
      <button onClick={() => setConfirm(true)}>Revoke</button>}</footer>
  </RecordDrawer>
}

function AssetLink({ item }: { item: Agent }) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [assetId, setAssetId] = useState('')
  const q = useDebounced(search.trim())
  const client = useQueryClient()
  const assets = useQuery({
    queryKey: ['assets', 'device-link', q],
    queryFn: ({ signal }) => request<AssetPage>('/assets?' + new URLSearchParams({ q, state: 'active' }), 'GET', undefined, signal),
    enabled: open && q.length >= 2, retry: false,
  })
  const save = useMutation({
    mutationFn: (id: string) => request('/agents/' + item.id + '/asset', 'PUT', { assetId: id || null }),
    onSuccess: async () => { setOpen(false); await client.invalidateQueries({ queryKey: ['agents', 'list'] }) },
  })
  return <div className="confirmation">
    {!open ? <button onClick={() => { setOpen(true); save.reset() }}>{item.assetId ? 'Change linked asset' : 'Link an existing asset'}</button> :
      <form onSubmit={(event) => { event.preventDefault(); if (assetId) save.mutate(assetId) }}>
        <label>Find active assets<input value={search} maxLength={120} onChange={(event) => { setSearch(event.target.value); setAssetId('') }} placeholder="Enter at least two characters" /></label>
        {assets.isFetching && <p role="status">Finding assets…</p>}
        {assets.isError && <p role="alert">{assets.error.message}</p>}
        {assets.data && <label>Matching asset<select value={assetId} onChange={(event) => setAssetId(event.target.value)}>
          <option value="">{assets.data.items.length ? 'Choose an asset' : 'No matches'}</option>
          {assets.data.items.map((asset) => <option key={asset.id} value={asset.id}>{asset.number} · {asset.name}</option>)}
        </select></label>}
        {save.isError && <p role="alert">{save.error.message}</p>}
        <button type="button" disabled={save.isPending} onClick={() => setOpen(false)}>Cancel</button>
        <button type="submit" disabled={!assetId || save.isPending}>Save link</button>
        {item.assetId && <button type="button" disabled={save.isPending} onClick={() => save.mutate('')}>Unlink asset</button>}
      </form>}
  </div>
}
