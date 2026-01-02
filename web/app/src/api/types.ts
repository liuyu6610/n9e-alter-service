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

export interface ProcessorDrop {
  when: string
}

export interface ProcessorRelabelRule {
  target: string
  pattern: string
  replace: string
}

export interface ProcessorRelabel {
  rules: ProcessorRelabelRule[]
}

export interface ProcessorUpdateSet {
  field: string
  value: string
}

export interface ProcessorUpdate {
  sets: ProcessorUpdateSet[]
}

export interface ProcessorConfig {
  type: string
  enabled: boolean
  drop?: ProcessorDrop
  relabel?: ProcessorRelabel
  update?: ProcessorUpdate
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

export interface WebhookConfig {
  enabled: boolean
  url: string
  timeout_seconds: number
  headers?: Record<string, string>
}

export interface SilenceRule {
  name: string
  enabled: boolean
  route_name: string
  tags?: Record<string, string>
  tag_regex?: Record<string, string>
  expires_at_unix: number
}

export interface EscalationConfig {
  after_seconds: number
  repeat_interval_seconds: number
  robot_ids: string[]
  webhook: WebhookConfig
}

export interface RuleRouteNotify {
  enabled: boolean
  dingtalk: DingTalkConfig
  webhook: WebhookConfig
  robot_id: string
  observe_seconds: number
  repeat_interval_seconds: number
  send_recovered: boolean
  escalations: EscalationConfig[]
}

export interface RuleRouteConfig {
  name: string
  enabled: boolean
  match: RouteMatch
  dedup: RouteDedup
  processors?: ProcessorConfig[]
  notify: RuleRouteNotify
  daily_report: RouteDailyReport
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
  processors?: ProcessorConfig[]
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
  tags?: Record<string, string>
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

export interface PreviewBindingItem {
  name: string
  priority: number
}

export interface PreviewItem {
  n9e_hash: string
  n9e_id: number
  group_id: number
  rule_id: number
  severity: number
  tags?: Record<string, string>

  route_name: string
  dedup_key: string
  service_hash: string

  group_name_before: string
  group_name_after: string
  rule_name_before: string
  rule_name_after: string
  entity_before: string
  entity_after: string

  matched_bindings?: PreviewBindingItem[]
  final_robot_ids?: string[]
}

export interface PreviewResponse {
  time: string
  items: PreviewItem[]
}

export interface N9EConfig {
  base_url: string
  api_path: string
  user_token: string
  authorization: string
  timeout_seconds: number
  verify_tls: boolean
}

export interface PullConfig {
  interval_seconds: number
  page_limit: number
  max_pages: number
  my_groups: boolean
  hours: number
  stime: number
  etime: number
  query: string
  severity: string
  prods: string
  rule_prods: string
  cate: string
  rid: number
  event_ids: string
}

export interface RedisConfig {
  enabled: boolean
  addr: string
  password: string
  db: number
  key_prefix: string
  hot_ttl_seconds: number
  ttl_seconds: number
}

export interface StateConfig {
  snapshot_file: string
  snapshot_interval_seconds: number
  retain_recovered_seconds: number
  recover_miss_count: number
  redis: RedisConfig
}

export interface PushConfig {
  enabled: boolean
  token: string
  queue_size: number
  worker_count: number
  enqueue_timeout_milli: number
}

export interface DingTalkConfig {
  webhook: string
  secret: string
  keyword: string
}

export interface RobotConfig {
  id: string
  webhook: string
  secret: string
  keyword: string
  fallback_robot_ids: string[]
}

export interface BindingRule {
  name: string
  priority: number
  enabled: boolean
  robot_id: string
  robot_ids: string[]
  group_id: number
  group_name_regex: string
  rule_id: number
  rule_name_regex: string
  route_name: string
  tags?: Record<string, string>
  tag_regex?: Record<string, string>
}

export interface RuleSet {
  n9e: N9EConfig
  pull: PullConfig
  push: PushConfig
  state: StateConfig
  silences: SilenceRule[]
  routes: RuleRouteConfig[]
  robots: RobotConfig[]
  bindings: BindingRule[]
  dingtalk: DingTalkConfig
}

export interface RulesCurrentResponse {
  time: string
  hash: string
  exists: boolean
  rules: RuleSet
}

export interface RulesPublishRequest {
  rules: RuleSet
  message: string
  actor: string
}

export interface RulesPublishResponse {
  ok: boolean
  time: string
  version: string
  hash: string
}

export interface RulesVersionsResponse {
  time: string
  items: string[]
}

export interface RulesVersionGetResponse {
  time: string
  version: string
  hash: string
  rules: RuleSet
}

export interface AuditRecord {
  at_unix: number
  action: string
  version: string
  hash: string
  message: string
  actor: string
}

export interface RulesAuditsResponse {
  time: string
  items: AuditRecord[]
}

export interface RulesRollbackRequest {
  version: string
  message: string
  actor: string
}

export interface RulesRollbackResponse {
  ok: boolean
  time: string
  version: string
  hash: string
}
