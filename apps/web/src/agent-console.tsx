import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, Eye, Monitor, Package, Plus, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { RecordDrawer } from './components/record-drawer'
import { useDebounced } from './lib/inventory'
import type { AssetPage } from './lib/inventory'
import { request, RequestError } from './lib/api'

type Agent = {
  id: string; deviceId: string; hostname: string; osFamily: string; osName?: string; serial?: string; architecture: string
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
type InventoryRun = { runId: string; scannedAt: string; receivedAt: string; agentVersion: string; softwareStatus?: string }
type Software = { id: string; source: string; name: string; version: string; publisher?: string }

const date = (value: string | null) => value ? new Date(value).toLocaleString() : '—'
const osLabels: Record<string, string> = { windows: 'Windows', linux: 'Linux', darwin: 'macOS' }

function AgentTimestamp({ value }: { value: string | null }) {
  return value ? <time dateTime={value} title={value}>{date(value)}</time> : '—'
}

export function Agents({ permissions }: { permissions: string[] }) {
  const [selected, setSelected] = useState<{ id: string; software?: boolean } | null>(null)
  const [page, setPage] = useState(1)
  const [downloadsOpen, setDownloadsOpen] = useState(false)
  const canEnroll = permissions.includes('agents.enroll')
  const list = useQuery({
    queryKey: ['agents', 'list', page],
    queryFn: ({ signal }) => request<{ items: Agent[]; hasNext: boolean }>('/agents?page=' + page, 'GET', undefined, signal),
    refetchInterval: 60_000, retry: false,
  })
  const agent = list.data?.items.find((item) => item.id === selected?.id)
  return <>
    <header className="page-heading">
      <div><h1>Endpoint agents</h1><p>Review enrolled devices and their latest reported inventory.</p></div>
      {canEnroll && <button className="button-primary" onClick={() => setDownloadsOpen(true)}><Download size={16} /> Agent downloads</button>}
    </header>
    {canEnroll && <AgentDownloads open={downloadsOpen} close={() => setDownloadsOpen(false)} />}
    <section className="table-panel" aria-label="Registered agents">
      <div className="table-toolbar"><button onClick={() => void list.refetch()} disabled={list.isFetching}><RefreshCw size={15} /> Refresh</button></div>
      {list.isPending && <p className="table-state" role="status">Loading agents…</p>}
      {list.isError && <p className="table-state" role="alert">{list.error.message}</p>}
      {list.data?.items.length === 0 && <div className="table-state"><Monitor size={24} /><h2>No agents enrolled</h2></div>}
      {!!list.data?.items.length && <div className="asset-table-wrap"><table className="asset-table">
        <thead><tr><th scope="col">Hostname</th><th scope="col">Device ID</th><th scope="col">Agent ID</th><th scope="col">Serial number</th><th scope="col">Operating system</th><th scope="col">Last seen</th><th scope="col">Device status</th><th scope="col">Last scan</th><th scope="col">Agent version</th><th scope="col">Actions</th></tr></thead>
        <tbody>{list.data.items.map((item) => <tr key={item.id}>
          <td><button className="text-button" onClick={() => setSelected({ id: item.id })}>{item.hostname || 'Awaiting first inventory'}</button></td>
          <td><code className="agent-table-id" title={item.deviceId}>{item.deviceId}</code></td>
          <td><code className="agent-table-id" title={item.id}>{item.id}</code></td>
          <td>{item.serial || '—'}</td>
          <td>{item.osName || osLabels[item.osFamily] || item.osFamily || '—'} {item.architecture}</td>
          <td><AgentTimestamp value={item.lastSeen} /></td>
          <td><span className={item.status === 'online' ? 'badge' : 'badge badge-muted'}>{item.status}</span></td>
          <td><AgentTimestamp value={item.lastInventoryAt} /></td><td>{item.version || '—'}</td>
          <td><div className="row-actions">
            <button onClick={() => setSelected({ id: item.id })}><Eye size={15} aria-hidden="true" /> View</button>
            {permissions.includes('software.read') && <button onClick={() => setSelected({ id: item.id, software: true })}><Package size={15} aria-hidden="true" /> Software</button>}
          </div></td>
        </tr>)}</tbody>
      </table></div>}
      {(page > 1 || list.data?.hasNext) && <div className="table-pagination">
        <button disabled={page === 1} onClick={() => setPage(page - 1)}>Previous</button>
        <span>Page {page}</span>
        <button disabled={!list.data?.hasNext} onClick={() => setPage(page + 1)}>Next</button>
      </div>}
    </section>
    {agent && <Details item={agent} permissions={permissions} showSoftware={selected?.software} close={() => setSelected(null)} />}
  </>
}

function AgentDownloads({ open, close }: { open: boolean; close: () => void }) {
  const [credential, setCredential] = useState<Enrollment>()
  const [copied, setCopied] = useState(false)
  const setup = useQuery({
    queryKey: ['agents', 'setup'],
    enabled: open,
    queryFn: ({ signal }) => request<{ origin: string; platforms: string[]; windowsInstaller: boolean; windowsInstallerTest: boolean; windowsInstallerBundledCA: boolean; publicCA: boolean }>('/agents/setup', 'GET', undefined, signal),
    retry: false,
  })
  const issue = useMutation({
    gcTime: 0, mutationFn: () => request<Enrollment>('/agents/enrollment-tokens', 'POST'),
    onSuccess: (value) => { setCredential(value); issue.reset() },
  })
  const revoke = useMutation({
    mutationFn: () => request('/agents/enrollment-tokens/' + credential?.id + '/revoke', 'POST'),
    onSuccess: () => { setCredential(undefined); setCopied(false) },
  })
  if (!open) return null
  return <RecordDrawer title="Agent downloads" description="Choose the package that matches the endpoint you manage." onClose={close}>
    <div className="drawer-body agent-downloads">
      {setup.isPending && <p role="status">Checking setup downloads…</p>}
      {setup.isError && <p role="alert">{setup.error.message} <button onClick={() => void setup.refetch()}>Retry</button></p>}
      {setup.data && !setup.data.windowsInstaller && setup.data.platforms.length === 0 &&
        <p>No downloads are configured for this server.</p>}
      {setup.data && <>
        {setup.data.origin && <p><strong>Server</strong> {setup.data.origin}</p>}
        {setup.data.origin.startsWith('http:') && <div className="confirmation">
          <p>This download is for a test agent on this computer.</p>
          <p>For a separate endpoint, open this console using the Cryol server’s LAN HTTPS address before downloading.</p>
        </div>}
        <div className="agent-download-grid">
          {setup.data.windowsInstaller ? <section aria-label="Windows setup">
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
          </section> : null}
          <UnixSetup platform="linux" available={setup.data.platforms} />
          <UnixSetup platform="macos" available={setup.data.platforms} />
        </div>
      </>}
      <div className="agent-enrollment">
        <h3>Enrollment token</h3>

        {credential && <>
          <p>The token expires at {date(credential.expiresAt)}.</p>
          <label>Enrollment token<input readOnly value={credential.token} spellCheck={false} onFocus={(event) => event.target.select()} /></label>
          {navigator.clipboard && <button onClick={() => {
            void navigator.clipboard.writeText(credential.token).then(() => setCopied(true), () => setCopied(false))
          }}>{copied ? 'Token copied' : 'Copy token'}</button>}
          {revoke.isError && <p role="alert">{revoke.error.message}</p>}
        </>}
        {issue.isError && <p role="alert">{issue.error.message}</p>}
        {credential
          ? <button className="danger-button" disabled={revoke.isPending} onClick={() => revoke.mutate()}>Revoke unused token</button>
          : <button className="button-primary" disabled={issue.isPending} onClick={() => issue.mutate()}><Plus size={16} /> Generate token</button>}
      </div>
    </div>
  </RecordDrawer>
}

function UnixSetup({ platform, available }: { platform: 'linux' | 'macos'; available: string[] }) {
  const packages = platform === 'macos'
    ? [{ id: 'macos-arm64', label: 'macOS Apple silicon' }, { id: 'macos-intel', label: 'macOS Intel' }]
    : [{ id: 'linux', label: 'Linux x64' }, { id: 'linux-arm64', label: 'Linux ARM64' }]
  const choices = packages.filter((item) => available.includes(item.id))
  if (!choices.length) return null
  const label = platform === 'macos' ? 'macOS' : 'Linux'
  return <section aria-label={label + ' setup'}>
    <h3>{label} setup</h3>
    <div className="row-actions">
      {choices.map((item) => <a key={item.id} className="button" href={'/api/v1/agents/setup/' + item.id}><Download size={16} /> Download {item.label} ZIP</a>)}
    </div>
    <p>Extract the ZIP and run <code>sh start-cryol-agent.sh</code>. Paste a token when prompted on first setup. Keep the terminal open for inventory updates.</p>
    {platform === 'macos' && <p>The macOS agent currently reports hardware, operating system and network inventory.</p>}
  </section>
}

function Details({ item, permissions, showSoftware, close }: { item: Agent; permissions: string[]; showSoftware?: boolean; close: () => void }) {
  const [confirm, setConfirm] = useState(false)
  const canReadHistory = permissions.includes('hardware.read') || permissions.includes('software.read')
  const [section, setSection] = useState<'hardware' | 'software' | 'history'>(showSoftware || !permissions.includes('hardware.read') ? 'software' : 'hardware')
  const hardwareTab = section === 'hardware'
  const softwareTab = section === 'software'
  const [historyPage, setHistoryPage] = useState(1)
  const [filter, setFilter] = useState('')
  const q = useDebounced(filter.trim())
  const [page, setPage] = useState(1)
  const client = useQueryClient()
  const base = '/agents/' + item.id
  const hardware = useQuery({
    queryKey: ['agents', item.id, 'hardware'],
    queryFn: ({ signal }) => request<Hardware>(base + '/inventory?part=hardware', 'GET', undefined, signal),
    enabled: hardwareTab && permissions.includes('hardware.read'), retry: false,
  })
  const software = useQuery({
    queryKey: ['agents', item.id, 'software', q, page],
    queryFn: ({ signal }) => request<{ software: Software[]; scannedAt: string; hasNext: boolean }>(base + '/inventory?' + new URLSearchParams({ part: 'software', q, page: String(page) }), 'GET', undefined, signal),
    enabled: softwareTab && permissions.includes('software.read'), retry: false,
  })
  const history = useQuery({
    queryKey: ['agents', item.id, 'history', historyPage],
    queryFn: ({ signal }) => request<{ items: InventoryRun[]; hasNext: boolean }>(base + '/inventory-history?page=' + historyPage, 'GET', undefined, signal),
    enabled: section === 'history' && canReadHistory, retry: false,
  })
  const revoke = useMutation({
    mutationFn: () => request(base + '/revoke', 'POST'),
    onSuccess: async () => { setConfirm(false); await client.invalidateQueries({ queryKey: ['agents', 'list'] }) },
  })
  const info = hardware.data
  const products = software.data?.software
  const error = hardwareTab ? hardware.error : softwareTab ? software.error : null
  return <RecordDrawer title={item.hostname || 'Endpoint agent'} description={item.id} onClose={close} wide>
    <div className="drawer-body">
      <p><strong>Status</strong> {item.status} · <strong>Last seen</strong> <AgentTimestamp value={item.lastSeen} /></p>
      <p><strong>Device ID</strong> <code>{item.deviceId}</code></p>
      {permissions.includes('assets.read') && <p><strong>Linked asset</strong> {item.assetNumber || 'None'}</p>}
      {permissions.includes('assets.read') && permissions.includes('devices.manage') && <AssetLink item={item} />}
      <div className="row-actions" aria-label="Inventory sections">
        {permissions.includes('hardware.read') && <button aria-pressed={hardwareTab} onClick={() => setSection('hardware')}>Hardware and network</button>}
        {permissions.includes('software.read') && <button aria-pressed={softwareTab} onClick={() => setSection('software')}>Installed software</button>}
        {canReadHistory && <button aria-pressed={section === 'history'} onClick={() => setSection('history')}>Scan history</button>}
      </div>
      {error && <p role="status">{error instanceof RequestError && error.status === 404 ? 'No scan is available for this section.' : error.message}</p>}
      {hardwareTab && hardware.isFetching && <p role="status">Loading hardware…</p>}
      {hardwareTab && info && <>
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
      {section === 'history' && canReadHistory && <>
        <h3>Reported scans</h3>
        {history.isPending && <p role="status">Loading scan history…</p>}
        {history.isError && <p role="alert">{history.error.message} <button onClick={() => void history.refetch()}>Retry</button></p>}
        {history.data && (history.data.items.length ? <div className="asset-table-wrap">
          <table className="asset-table">
            <thead><tr><th scope="col">Scanned</th><th scope="col">Received</th><th scope="col">Agent version</th>
              {permissions.includes('software.read') && <th scope="col">Software collection</th>}</tr></thead>
            <tbody>{history.data.items.map((run) => <tr key={run.runId}>
              <td><AgentTimestamp value={run.scannedAt} /></td><td><AgentTimestamp value={run.receivedAt} /></td>
              <td>{run.agentVersion || '—'}</td>
              {permissions.includes('software.read') && <td>{run.softwareStatus || '—'}</td>}
            </tr>)}</tbody>
          </table>
        </div> : <p>No scans have been reported.</p>)}
        {(historyPage > 1 || history.data?.hasNext) && <div className="table-pagination">
          <button disabled={historyPage === 1} onClick={() => setHistoryPage(historyPage - 1)}>Previous</button>
          <span>Page {historyPage}</span>
          <button disabled={!history.data?.hasNext} onClick={() => setHistoryPage(historyPage + 1)}>Next</button>
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
