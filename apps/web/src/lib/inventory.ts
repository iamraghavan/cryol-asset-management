import { useEffect, useState } from 'react'

export const dashboardKey = ['dashboard-summary']
export type AssetFields = {
  name: string; serial: string; manufacturer: string; model: string; notes: string; purchaseDate: string
}
export type Asset = AssetFields & {
  id: string; number: string; createdAt: string; archivedAt: string | null; revision: number
}
export type AssetDetails = Asset & { history: { action: string; at: string }[] | null }
export type AssetPage = { items: Asset[]; hasNext: boolean; page: number }

export function useDebounced<T>(value: T, delay = 250) {
  const [current, setCurrent] = useState(value)
  useEffect(() => {
    const timeout = window.setTimeout(() => setCurrent(value), delay)
    return () => window.clearTimeout(timeout)
  }, [value, delay])
  return current
}
