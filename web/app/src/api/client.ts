import type {
  AlertGetResponse,
  AlertsResponse,
  PreviewResponse,
  RulesAuditsResponse,
  RulesCurrentResponse,
  RulesPublishRequest,
  RulesPublishResponse,
  RulesRollbackRequest,
  RulesRollbackResponse,
  RulesVersionGetResponse,
  RulesVersionsResponse,
  RoutesResponse,
  StatusResponse,
} from './types'

async function requestJson<T>(input: RequestInfo | URL, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers || {})
  const tok = (typeof window !== 'undefined' ? window.localStorage.getItem('api_token') : '') || ''
  if (tok.trim().length > 0 && !headers.has('Authorization') && !headers.has('authorization')) {
    headers.set('Authorization', `Bearer ${tok.trim()}`)
  }
  const resp = await fetch(input, { cache: 'no-store', ...init, headers })
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

export function previewRoutes(payload: any): Promise<PreviewResponse> {
  return requestJson<PreviewResponse>('/api/v1/routes/preview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export function getRulesCurrent(): Promise<RulesCurrentResponse> {
  return requestJson<RulesCurrentResponse>('/api/v1/rules/current')
}

export function publishRules(payload: RulesPublishRequest): Promise<RulesPublishResponse> {
  return requestJson<RulesPublishResponse>('/api/v1/rules/publish', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export function listRulesVersions(): Promise<RulesVersionsResponse> {
  return requestJson<RulesVersionsResponse>('/api/v1/rules/versions')
}

export function getRulesVersion(version: string): Promise<RulesVersionGetResponse> {
  const usp = new URLSearchParams()
  usp.set('version', version)
  return requestJson<RulesVersionGetResponse>(`/api/v1/rules/version?${usp.toString()}`)
}

export function listRulesAudits(limit = 50): Promise<RulesAuditsResponse> {
  const usp = new URLSearchParams()
  usp.set('limit', String(limit))
  return requestJson<RulesAuditsResponse>(`/api/v1/rules/audits?${usp.toString()}`)
}

export function rollbackRules(payload: RulesRollbackRequest): Promise<RulesRollbackResponse> {
  return requestJson<RulesRollbackResponse>('/api/v1/rules/rollback', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function getHealth(): Promise<{ status: string; time: string }> {
  return requestJson<{ status: string; time: string }>('/healthz')
}

export async function getReady(): Promise<{ status: string; time: string }> {
  return requestJson<{ status: string; time: string }>('/readyz')
}
