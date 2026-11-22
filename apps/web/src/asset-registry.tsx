import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table'
import type { ColumnDef, RowSelectionState, VisibilityState } from '@tanstack/react-table'
import { Archive, Boxes, Columns3, Download, ChevronLeft, ChevronRight, Eye, ListFilter, Pencil, Plus, Search } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useBlocker, useSearchParams } from 'react-router'
import { RecordDrawer } from './components/record-drawer'
import { request } from './lib/api'
import { dashboardKey, useDebounced } from './lib/inventory'
import type { Asset, AssetDetails, AssetFields, AssetPage } from './lib/inventory'

const assetFields = z.object({
  name: z.string().trim().min(1, 'Enter an asset name.').max(200),
  serial: z.string().trim().max(120),
  manufacturer: z.string().trim().max(120),
  model: z.string().trim().max(120),
  notes: z.string().trim().max(4000),
  purchaseDate: z.union([z.literal(''), z.iso.date()]),
})

const defaults: AssetFields = { name: '', serial: '', manufacturer: '', model: '', purchaseDate: '', notes: '' }
const emptyAssets: Asset[] = []
const assetDetailKey = (id: string) => ['assets', 'detail', id]

export function Assets({ permissions }: { permissions: string[] }) {
  const [params, setParams] = useSearchParams()
  const [message, setMessage] = useState('')
  const [filterOpen, setFilterOpen] = useState(false)
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
  const [selection, setSelection] = useState<{ scope: string; rows: RowSelectionState }>({ scope: '', rows: {} })
  const q = params.get('q') ?? ''
  const state = params.get('state') ?? ''
  const sort = params.get('sort') ?? ''
  const manufacturer = params.get('manufacturer') ?? ''
  const from = params.get('from') ?? ''
  const to = params.get('to') ?? ''
  const hasFilters = Boolean(state || sort || manufacturer || from || to)
  const search = useDebounced(q.trim())
  const page = Math.max(1, Number(params.get('page')) || 1)
  const selectionScope = JSON.stringify([search, state, sort, manufacturer, from, to, page])
  const rowSelection = selection.scope === selectionScope ? selection.rows : {}
  const detailID = params.get('asset') ?? ''
  const mode = params.get('mode')
  const canCreate = permissions.includes('assets.create')
  const canEdit = permissions.includes('assets.update')
  const canArchive = permissions.includes('assets.archive')
  const client = useQueryClient()

  const list = useQuery({
    queryKey: ['assets', 'list', search, state, sort, manufacturer, from, to, page],
    queryFn: ({ signal }) => request<AssetPage>('/assets?' + new URLSearchParams({ q: search, state, sort, manufacturer, from, to, page: String(page) }), 'GET', undefined, signal),
    retry: false,
  })
  const detail = useQuery({
    queryKey: assetDetailKey(detailID),
    queryFn: ({ signal }) => request<AssetDetails>('/assets/' + encodeURIComponent(detailID), 'GET', undefined, signal),
    enabled: Boolean(detailID), retry: false,
  })

  function update(values: Record<string, string>, replace = false) {
    setParams((current) => {
      for (const [key, value] of Object.entries(values)) {
        if (value) current.set(key, value)
        else current.delete(key)
      }
      return current
    }, { replace, preventScrollReset: true })
  }
  function closeDrawer() { update({ asset: '', mode: '' }, true) }
  async function saved(result: Asset, action: string) {
    setMessage(`${result.number} ${action}.`)
    await Promise.all([
      client.invalidateQueries({ queryKey: ['assets'] }),
      client.invalidateQueries({ queryKey: dashboardKey }),
    ])
  }

  const columns = useMemo<ColumnDef<Asset>[]>(() => [
    { accessorKey: 'number', header: 'Asset ID', cell: ({ row }) => <button className="text-button" onClick={() => {
      setParams((current) => {
        current.set('asset', row.original.id)
        current.delete('mode')
        return current
      }, { preventScrollReset: true })
    }}>{row.original.number}</button> },
    { accessorKey: 'name', header: 'Asset name' },
    { accessorKey: 'manufacturer', header: 'Manufacturer', cell: ({ getValue }) => getValue<string>() || '—' },
    { accessorKey: 'model', header: 'Model', cell: ({ getValue }) => getValue<string>() || '—' },
    { accessorKey: 'serial', header: 'Serial number', cell: ({ getValue }) => getValue<string>() || '—' },
    { accessorKey: 'purchaseDate', header: 'Purchase date', cell: ({ getValue }) => getValue<string>() || '—' },
    { accessorKey: 'archivedAt', header: 'Status', cell: ({ row }) => <AssetStatus asset={row.original} /> },
  ], [setParams])
  const table = useReactTable({
    data: list.data?.items ?? emptyAssets, columns, getCoreRowModel: getCoreRowModel(), getRowId: (asset) => asset.id,
    state: { rowSelection, columnVisibility }, onColumnVisibilityChange: setColumnVisibility,
    onRowSelectionChange: (change) => setSelection({ scope: selectionScope, rows: typeof change === 'function' ? change(rowSelection) : change }),
  })
  const chosen = table.getSelectedRowModel().rows.map((row) => row.original)

  const record = detail.data
  const showCreate = mode === 'create' && canCreate && !detailID
  const showEdit = mode === 'edit' && canEdit && record && !record.archivedAt
  return (
    <>
      <header className="page-heading">
        <div><h1>Asset registry</h1><p>Find and manage the assets registered to your organization.</p></div>
        {canCreate && <button className="button-primary" onClick={() => update({ mode: 'create', asset: '' })}><Plus size={17} /> Add asset</button>}
      </header>
      <section className="table-panel" aria-label="Asset list">
        <div className="table-toolbar">
          <label className="search-field"><Search size={17} /><input aria-label="Search asset registry" placeholder="Search by ID, name or serial number" maxLength={120} value={q} onChange={(event) => update({ q: event.target.value, page: '' }, true)} /></label>
          <button onClick={() => setFilterOpen(true)}><ListFilter size={16} /> Filters{hasFilters && ' •'}</button>
          <details className="column-menu">
            <summary className="button"><Columns3 size={16} /> Columns</summary>
            <div>{table.getAllLeafColumns().filter((column) => !['number', 'name', 'archivedAt'].includes(column.id)).map((column) =>
              <label key={column.id}><input type="checkbox" checked={column.getIsVisible()} onChange={column.getToggleVisibilityHandler()} />{String(column.columnDef.header)}</label>
            )}</div>
          </details>
          {state && <button className="filter-chip" aria-label="Clear status filter" onClick={() => update({ state: '', page: '' })}>{state === 'archived' ? 'Archived' : 'Active'} ×</button>}
        </div>
        {chosen.length > 0 && <div className="selection-toolbar">
          <span>{chosen.length} selected on this page</span>
          <button onClick={() => exportSelected(chosen)}><Download size={16} /> Export CSV</button>
          <button onClick={() => table.resetRowSelection(true)}>Clear selection</button>
        </div>}
        {message && <p className="success-message" role="status">{message}</p>}
        {list.isPending && <p className="table-state" role="status">Loading assets…</p>}
        {list.isError && <p className="table-state" role="alert">{list.error.message} <button onClick={() => void list.refetch()}>Retry</button></p>}
        {list.data?.items.length === 0 && <div className="table-state"><Boxes size={23} /><h2>{q || hasFilters ? 'No matching assets' : 'No assets registered'}</h2>
          {q || hasFilters ? <button onClick={() => update({ q: '', state: '', sort: '', manufacturer: '', from: '', to: '', page: '' })}>Clear search and filters</button> : canCreate && <button onClick={() => update({ mode: 'create' })}>Register an asset</button>}
        </div>}
        {table.getRowModel().rows.length > 0 && (
          <div className="asset-table-wrap">
            <table className="asset-table">
              <thead>{table.getHeaderGroups().map((group) => <tr key={group.id}><th scope="col"><input aria-label="Select all assets on this page" type="checkbox" checked={table.getIsAllPageRowsSelected()} onChange={table.getToggleAllPageRowsSelectedHandler()} /></th>{group.headers.map((header) => <th scope="col" key={header.id}>{flexRender(header.column.columnDef.header, header.getContext())}</th>)}<th scope="col">Actions</th></tr>)}</thead>
              <tbody>{table.getRowModel().rows.map((row) => <tr key={row.original.id}>
                <td><input aria-label={`Select ${row.original.number}`} type="checkbox" checked={row.getIsSelected()} onChange={row.getToggleSelectedHandler()} /></td>
                {row.getVisibleCells().map((cell) => <td key={cell.id}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</td>)}
                <td><div className="row-actions">
                  <button className="icon-button" aria-label={`View ${row.original.number}`} title="View asset" onClick={() => update({ asset: row.original.id, mode: '' })}><Eye size={16} /></button>
                  {canEdit && !row.original.archivedAt && <button className="icon-button" aria-label={`Edit ${row.original.number}`} title="Edit asset" onClick={() => update({ asset: row.original.id, mode: 'edit' })}><Pencil size={15} /></button>}
                </div></td>
              </tr>)}</tbody>
            </table>
          </div>
        )}
        {(page > 1 || list.data?.hasNext) && <div className="table-pagination">
          <span>Page {page}</span>
          <button className="icon-button" aria-label="Previous page" disabled={page === 1} onClick={() => update({ page: String(page - 1) })}><ChevronLeft size={17} /></button>
          <button className="icon-button" aria-label="Next page" disabled={!list.data?.hasNext} onClick={() => update({ page: String(page + 1) })}><ChevronRight size={17} /></button>
        </div>}
      </section>
      {filterOpen && <FilterDrawer filters={{ state, sort, manufacturer, from, to }} onClose={() => setFilterOpen(false)} onApply={(values) => {
        update({ ...values, page: '' })
        setFilterOpen(false)
      }} />}
      {(showCreate || showEdit) && <AssetEditor key={record?.id ?? 'create'} asset={showEdit ? record : undefined} onClose={closeDrawer} onSaved={async (result) => {
        closeDrawer()
        await saved(result, showEdit ? 'updated' : 'created')
      }} />}
      {detailID && !showEdit && <RecordDrawer title={record?.name ?? 'Asset details'} description={record?.number ?? 'Registry record'} onClose={closeDrawer}>
        {detail.isPending && <p className="drawer-body" role="status">Loading details…</p>}
        {detail.isError && <p className="drawer-body" role="alert">{detail.error.message} <button onClick={() => void detail.refetch()}>Retry</button></p>}
        {record && <AssetView asset={record} canEdit={canEdit} canArchive={canArchive} onEdit={() => update({ mode: 'edit' }, true)}
          onChanged={(result) => saved(result, result.archivedAt ? 'archived' : 'restored')} />}
      </RecordDrawer>}
    </>
  )
}

type AssetFilters = { state: string; sort: string; manufacturer: string; from: string; to: string }

function FilterDrawer({ filters, onClose, onApply }: {
  filters: AssetFilters; onClose: () => void; onApply: (values: AssetFilters) => void
}) {
  const [error, setError] = useState('')
  return <RecordDrawer title="Filter assets" description="Narrow the registry by status, manufacturer or purchase date." onClose={onClose}>
    <form className="drawer-form" onSubmit={(event) => {
      event.preventDefault()
      const data = new FormData(event.currentTarget)
      const values = {
        state: String(data.get('state')), sort: String(data.get('sort')),
        manufacturer: String(data.get('manufacturer')).trim(), from: String(data.get('from')), to: String(data.get('to')),
      }
      if (values.from && values.to && values.from > values.to) {
        setError('The end date must be on or after the start date.')
        return
      }
      onApply(values)
    }}>
      <div className="drawer-body field-grid">
        <label>Status<select name="state" defaultValue={filters.state}><option value="">All statuses</option><option value="active">Active</option><option value="archived">Archived</option></select></label>
        <label>Sort by<select name="sort" defaultValue={filters.sort}><option value="">Newest first</option><option value="oldest">Oldest first</option><option value="name">Asset name</option><option value="number">Asset ID</option></select></label>
        <label className="field-wide">Manufacturer<input name="manufacturer" maxLength={120} defaultValue={filters.manufacturer} /></label>
        <label>Purchased from<input name="from" type="date" defaultValue={filters.from} /></label>
        <label>Purchased through<input name="to" type="date" defaultValue={filters.to} /></label>
        {error && <p className="field-error field-wide" role="alert">{error}</p>}
      </div>
      <footer className="drawer-footer"><button type="button" onClick={() => onApply({ state: '', sort: '', manufacturer: '', from: '', to: '' })}>Reset filters</button><button className="button-primary">Apply filters</button></footer>
    </form>
  </RecordDrawer>
}

function AssetEditor({ asset, onClose, onSaved }: { asset?: Asset; onClose: () => void; onSaved: (asset: Asset) => Promise<void> }) {
  const [discard, setDiscard] = useState(false)
  const submitting = useRef(false)
  const allowNavigation = useRef(false)
  const form = useForm<AssetFields>({ resolver: zodResolver(assetFields), defaultValues: asset ?? defaults })
  const { isDirty, errors } = form.formState
  const blocker = useBlocker(() => (isDirty || submitting.current) && !allowNavigation.current)
  const mustConfirm = discard || blocker.state === 'blocked'
  const save = useMutation({
    mutationFn: (data: AssetFields) => asset
      ? request<Asset>('/assets/' + asset.id, 'PATCH', { ...data, revision: asset.revision })
      : request<Asset>('/assets', 'POST', data),
    onSuccess: (item) => {
      allowNavigation.current = true
      return onSaved(item)
    },
  })
  useEffect(() => {
    if (!isDirty) return
    const warn = (event: BeforeUnloadEvent) => event.preventDefault()
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [isDirty])
  function tryClose() {
    if (submitting.current || save.isPending) return
    if (isDirty) setDiscard(true)
    else onClose()
  }
  return <RecordDrawer title={asset ? 'Edit asset' : 'Add asset'} description={asset?.number ?? 'Cryol assigns the asset ID when you save.'} onClose={tryClose}>
    <form className="drawer-form" onSubmit={form.handleSubmit(async (values) => {
      if (submitting.current) return
      submitting.current = true
      try { await save.mutateAsync(values) } catch { /* The mutation exposes its error in the form. */ }
      finally { submitting.current = false }
    })}>
      <div className="drawer-body">
        <fieldset className="field-grid" disabled={save.isPending || mustConfirm}>
          <legend>Asset information</legend>
          <label className="field-wide">Asset name <input {...form.register('name')} required maxLength={200} aria-invalid={Boolean(errors.name)} />
            {errors.name?.message && <small className="field-error">{errors.name.message}</small>}
          </label>
          <label>Manufacturer<input {...form.register('manufacturer')} maxLength={120} /></label>
          <label>Model<input {...form.register('model')} maxLength={120} /></label>
          <label>Serial number<input {...form.register('serial')} maxLength={120} /></label>
          <label>Purchase date<input type="date" {...form.register('purchaseDate')} aria-invalid={Boolean(errors.purchaseDate)} />
            {errors.purchaseDate && <small className="field-error">Enter a valid purchase date.</small>}
          </label>
          <label className="field-wide">Notes<textarea {...form.register('notes')} maxLength={4000} rows={4} /></label>
        </fieldset>
        {save.isError && <p role="alert" className="field-error">{save.error.message}</p>}
        {mustConfirm && !save.isPending && <div role="alert" className="confirmation">
          <strong>Discard unsaved changes?</strong><p>Your changes will be lost.</p>
          <button type="button" onClick={() => {
            setDiscard(false)
            if (blocker.state === 'blocked') blocker.reset()
          }}>Keep editing</button>
          <button type="button" className="danger-button" onClick={() => {
            allowNavigation.current = true
            if (blocker.state === 'blocked') blocker.proceed()
            else onClose()
          }}>Discard changes</button>
        </div>}
      </div>
      <footer className="drawer-footer">
        <button type="button" disabled={save.isPending} onClick={tryClose}>Cancel</button>
        <button className="button-primary" disabled={save.isPending || mustConfirm}>{save.isPending ? 'Saving…' : 'Save asset'}</button>
      </footer>
    </form>
  </RecordDrawer>
}

const eventLabels: Record<string, string> = {
  'asset.created': 'Asset registered', 'asset.updated': 'Asset details updated',
  'asset.archived': 'Asset archived', 'asset.restored': 'Asset restored',
}

function AssetStatus({ asset }: { asset: Asset }) {
  return <span className={asset.archivedAt ? 'badge badge-muted' : 'badge'}>{asset.archivedAt ? 'Archived' : 'Active'}</span>
}

function AssetView({ asset, canEdit, canArchive, onEdit, onChanged }: {
  asset: AssetDetails; canEdit: boolean; canArchive: boolean; onEdit: () => void; onChanged: (asset: Asset) => Promise<void>
}) {
  const [confirm, setConfirm] = useState(false)
  const update = useMutation({
    mutationFn: () => request<Asset>('/assets/' + asset.id + (asset.archivedAt ? '/restore' : '/archive'), 'POST', { revision: asset.revision }),
    onSuccess: async (item) => { setConfirm(false); await onChanged(item) },
  })
  return <>
    <div className="drawer-body">
      <AssetStatus asset={asset} />
      <h3>Overview</h3>
      <dl className="asset-details">
        {([['Manufacturer', asset.manufacturer], ['Model', asset.model], ['Serial number', asset.serial], ['Purchase date', asset.purchaseDate], ['Notes', asset.notes]]).map(([key, value]) =>
          <div key={key}><dt>{key}</dt><dd>{value || '—'}</dd></div>)}
      </dl>
      <h3>Activity history</h3>
      {asset.history?.length ? <ol className="activity-log">{asset.history.map((event, i) => <li key={`${event.at}-${i}`}>
        <span>{eventLabels[event.action] ?? event.action}</span><time dateTime={event.at}>{new Date(event.at).toLocaleString()}</time>
      </li>)}</ol> : <p>No activity recorded.</p>}
      {confirm && <div className="confirmation">
        <strong>{asset.archivedAt ? 'Restore this asset?' : 'Archive this asset?'}</strong>
        <p>{asset.archivedAt ? 'Restoring returns the record to the active inventory.' : 'Archiving keeps the asset and its activity history and removes it from the active inventory.'}</p>
        <button disabled={update.isPending} onClick={() => setConfirm(false)}>Cancel</button>
        <button className={asset.archivedAt ? 'button-primary' : 'danger-button'} disabled={update.isPending} onClick={() => update.mutate()}>{asset.archivedAt ? 'Restore asset' : 'Archive asset'}</button>
        {update.isError && <p className="field-error" role="alert">{update.error.message}</p>}
      </div>}
    </div>
    <footer className="drawer-footer">
      {canArchive && <button disabled={confirm || update.isPending} onClick={() => { update.reset(); setConfirm(true) }}><Archive size={16} /> {asset.archivedAt ? 'Restore' : 'Archive'}</button>}
      {canEdit && !asset.archivedAt && <button className="button-primary" disabled={confirm || update.isPending} onClick={onEdit}><Pencil size={15} /> Edit asset</button>}
    </footer>
  </>
}

function exportSelected(assets: Asset[]) {
  function csvCell(value: string) {
    const safe = /^[\t\r\n]/.test(value) || /^[=+@-]/.test(value.trimStart()) ? "'" + value : value
    return '"' + safe.replaceAll('"', '""') + '"'
  }
  const rows = [['Asset ID', 'Asset name', 'Manufacturer', 'Model', 'Serial number', 'Purchase date', 'Status'],
    ...assets.map((asset) => [asset.number, asset.name, asset.manufacturer, asset.model, asset.serial, asset.purchaseDate, asset.archivedAt ? 'Archived' : 'Active'])]
  const file = new Blob([rows.map((row) => row.map(csvCell).join(',')).join('\r\n')], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(file)
  const link = document.createElement('a')
  link.href = url
  link.download = 'cryol-assets.csv'
  link.click()
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}
