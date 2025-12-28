import type {
  AlertGetResponse,
  AlertsResponse,
  RoutesResponse,
  StatusResponse,
} from './types'

async function requestJson<T>(input: RequestInfo | URL, init?: RequestInit): Promise<T> {
  const resp = await fetch(input, { cache: 'no-store', ...init })
  const data = (await resp.json().catch(() => ({}))) as any
  if (!resp.ok) {
    const msg = data && data.error ? String(data.error) : `http ${resp.status}`
    throw new Error(msg)
  }
  return data as T
}

export function getStatus(): Promise<StatusResponse> {
  return requestJson<StatusResponse>('/api/v1/status')
}

export function runPull(): Promise<any> {
  return requestJson<any>('/api/v1/pull/run', { method: 'POST' })
}

export function getRoutes(): Promise<RoutesResponse> {
  return requestJson<RoutesResponse>('/api/v1/routes')
}

export function listAlerts(params: {
  status: string
  route?: string
  offset: number
  limit: number
}): Promise<AlertsResponse> {
  const usp = new URLSearchParams()
  usp.set('status', params.status)
  usp.set('offset', String(params.offset))
  usp.set('limit', String(params.limit))
  if (params.route) usp.set('route', params.route)
  return requestJson<AlertsResponse>(`/api/v1/alerts?${usp.toString()}`)
}

export function getAlert(hash: string): Promise<AlertGetResponse> {
  const usp = new URLSearchParams()
  usp.set('hash', hash)
  return requestJson<AlertGetResponse>(`/api/v1/alerts/get?${usp.toString()}`)
}

export async function getHealth(): Promise<{ status: string; time: string }> {
  return requestJson<{ status: string; time: string }>('/healthz')
}

export async function getReady(): Promise<{ status: string; time: string }> {
  return requestJson<{ status: string; time: string }>('/readyz')
}
