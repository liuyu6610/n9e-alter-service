export type AlertStatus = 'active' | 'recovered'

export interface EngineStatus {
  last_start_unix: number
  last_end_unix: number
  last_error: string
  last_fetched: number
  last_total: number
  last_inputs: number
  active_total: number
  recovered_total: number
  new_actives: number
  new_recovereds: number
  purged_recovered: number
}

export interface StateSummary {
  active: number
  recovered: number
  total: number
}

export interface StatusResponse {
  time: string
  engine: EngineStatus
  state: StateSummary
}

export interface RouteMatch {
  group_name_regex: string
  rule_name_regex: string
  severity_in: number[] | null
}

export interface RewriteRule {
  field: string
  pattern: string
  replace: string
}

export interface RouteDedup {
  mode: string
  include_group_id: boolean
  include_group_name: boolean
  include_rule_id: boolean
  include_rule_name: boolean
  include_severity: boolean
  include_entity: boolean
  normalize_pod_name: boolean
  rewrites: RewriteRule[] | null
}

export interface RouteNotify {
  enabled: boolean
  observe_seconds: number
  repeat_interval_seconds: number
  send_recovered: boolean
}

export interface RouteDailyReport {
  enabled: boolean
  cron: string
  title_prefix: string
  max_lines: number
  max_chars: number
  clear_mode: string
}

export interface RouteItem {
  name: string
  enabled: boolean
  match: RouteMatch
  dedup: RouteDedup
  notify: RouteNotify
  daily_report: RouteDailyReport
}

export interface RoutesResponse {
  time: string
  items: RouteItem[]
}

export interface RecordItem {
  service_hash: string
  route_name: string
  dedup_key: string
  status: AlertStatus
  first_seen_at: number
  last_seen_at: number
  miss_count: number
  recovered_at: number
  last_notified: number
  n9e_hash: string
  n9e_id: number
  group_id: number
  group_name: string
  rule_id: number
  rule_name: string
  severity: number
  entity: string
  first_trigger_time: number
  last_trigger_time: number
  raw_count: number
}

export interface AlertsResponse {
  time: string
  total: number
  offset: number
  limit: number
  items: RecordItem[]
}

export interface AlertGetResponse {
  time: string
  item: RecordItem
}
