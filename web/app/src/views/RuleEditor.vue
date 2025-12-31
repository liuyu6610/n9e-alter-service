<template>
  <el-row :gutter="12">
    <el-col :span="6">
      <el-card shadow="never">
        <template #header>
          <div style="display: flex; justify-content: space-between; align-items: center">
            <div>规则（Routes）</div>
            <div style="display: flex; gap: 8px">
              <el-button size="small" :loading="loading" @click="refresh">刷新</el-button>
              <el-button size="small" type="primary" @click="addRoute">新增</el-button>
            </div>
          </div>
        </template>

        <el-input v-model="filterText" placeholder="过滤 name" size="small" clearable style="margin-bottom: 10px" />

        <el-scrollbar height="520">
          <el-menu :default-active="activeName" @select="onSelect">
            <el-menu-item v-for="r in filteredRoutes" :key="r.name" :index="r.name">
              <div style="display: flex; justify-content: space-between; width: 100%; align-items: center">
                <span style="max-width: 120px; overflow: hidden; text-overflow: ellipsis">{{ r.name }}</span>
                <el-tag size="small" :type="r.enabled ? 'success' : 'info'">{{ r.enabled ? 'on' : 'off' }}</el-tag>
              </div>
            </el-menu-item>
          </el-menu>
        </el-scrollbar>

        <el-divider />

        <div style="display: flex; gap: 8px; flex-wrap: wrap">
          <el-button size="small" :disabled="!current" @click="cloneRoute">复制</el-button>
          <el-button size="small" type="danger" :disabled="!current" @click="removeRoute">删除</el-button>
          <el-button size="small" :disabled="routes.length === 0" @click="exportJson">导出 JSON</el-button>
          <el-button size="small" @click="openImport">导入 JSON</el-button>
        </div>
      </el-card>
    </el-col>

    <el-col :span="18">
      <el-card shadow="never">
        <template #header>
          <div style="display: flex; justify-content: space-between; align-items: center">
            <div>规则编辑器（体验版）</div>
            <div style="display: flex; gap: 8px; align-items: center">
              <el-tag type="warning">当前仅编辑本地副本，需导出后写回配置并重启生效</el-tag>
              <el-button type="success" :disabled="!current" @click="openPreview">样本预览</el-button>
            </div>
          </div>
        </template>

        <el-alert v-if="error" :title="error" type="error" show-icon style="margin-bottom: 10px" />

        <el-alert
          v-if="validationErrors.length > 0"
          title="校验失败（需修复后再导出落盘）"
          type="error"
          show-icon
          style="margin-bottom: 10px"
        >
          <template #default>
            <div v-for="(m, idx) in validationErrors" :key="idx">{{ m }}</div>
          </template>
        </el-alert>

        <el-alert
          v-if="validationWarnings.length > 0"
          title="校验告警（可继续，但建议处理）"
          type="warning"
          show-icon
          style="margin-bottom: 10px"
        >
          <template #default>
            <div v-for="(m, idx) in validationWarnings" :key="idx">{{ m }}</div>
          </template>
        </el-alert>

        <div v-if="!current" style="color: #666">请选择或新增一个路由规则</div>

        <div v-else>
          <el-form label-width="140px" label-position="left">
            <el-form-item label="name">
              <el-input v-model="current.name" placeholder="route name" />
            </el-form-item>
            <el-form-item label="enabled">
              <el-switch v-model="current.enabled" />
            </el-form-item>
          </el-form>

          <el-tabs v-model="activeTab" type="border-card">
            <el-tab-pane label="Match" name="match">
              <el-form label-width="160px" label-position="left">
                <el-form-item label="group_name_regex">
                  <el-input v-model="current.match.group_name_regex" placeholder=".*" />
                </el-form-item>
                <el-form-item label="rule_name_regex">
                  <el-input v-model="current.match.rule_name_regex" placeholder=".*" />
                </el-form-item>
                <el-form-item label="severity_in">
                  <el-select v-model="severityIn" multiple filterable clearable placeholder="例如: 1,2,3">
                    <el-option v-for="n in [1,2,3,4]" :key="n" :label="String(n)" :value="n" />
                  </el-select>
                </el-form-item>
              </el-form>
            </el-tab-pane>

            <el-tab-pane label="Dedup" name="dedup">
              <el-form label-width="200px" label-position="left">
                <el-form-item label="mode">
                  <el-select v-model="current.dedup.mode" filterable>
                    <el-option label="n9e_hash" value="n9e_hash" />
                    <el-option label="fields" value="fields" />
                  </el-select>
                </el-form-item>
                <el-form-item label="normalize_pod_name">
                  <el-switch v-model="current.dedup.normalize_pod_name" />
                </el-form-item>
                <el-form-item label="include_group_id"><el-switch v-model="current.dedup.include_group_id" /></el-form-item>
                <el-form-item label="include_group_name"><el-switch v-model="current.dedup.include_group_name" /></el-form-item>
                <el-form-item label="include_rule_id"><el-switch v-model="current.dedup.include_rule_id" /></el-form-item>
                <el-form-item label="include_rule_name"><el-switch v-model="current.dedup.include_rule_name" /></el-form-item>
                <el-form-item label="include_severity"><el-switch v-model="current.dedup.include_severity" /></el-form-item>
                <el-form-item label="include_entity"><el-switch v-model="current.dedup.include_entity" /></el-form-item>
              </el-form>
            </el-tab-pane>

            <el-tab-pane label="Rewrites" name="rewrites">
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px">
                <div style="font-weight: 600">rewrites</div>
                <el-button size="small" type="primary" @click="addRewrite">新增 rewrite</el-button>
              </div>

              <el-table :data="current.dedup.rewrites || []" size="small" border style="width: 100%">
                <el-table-column label="field" width="140">
                  <template #default="scope">
                    <el-select v-model="scope.row.field" size="small" style="width: 120px">
                      <el-option label="group_name" value="group_name" />
                      <el-option label="rule_name" value="rule_name" />
                      <el-option label="entity" value="entity" />
                    </el-select>
                  </template>
                </el-table-column>
                <el-table-column label="pattern">
                  <template #default="scope">
                    <el-input v-model="scope.row.pattern" size="small" placeholder="regex" />
                  </template>
                </el-table-column>
                <el-table-column label="replace">
                  <template #default="scope">
                    <el-input v-model="scope.row.replace" size="small" placeholder="replace" />
                  </template>
                </el-table-column>
                <el-table-column label="op" width="90">
                  <template #default="scope">
                    <el-button size="small" type="danger" @click="removeRewrite(scope.$index)">删除</el-button>
                  </template>
                </el-table-column>
              </el-table>
            </el-tab-pane>

            <el-tab-pane label="Notify" name="notify">
              <el-form label-width="220px" label-position="left">
                <el-form-item label="enabled"><el-switch v-model="current.notify.enabled" /></el-form-item>
                <el-form-item label="robot_id (route 默认)">
                  <el-input v-model="(current.notify as any).robot_id" placeholder="robot id" />
                </el-form-item>
                <el-form-item label="observe_seconds">
                  <el-input-number v-model="current.notify.observe_seconds" :min="0" :step="5" />
                </el-form-item>
                <el-form-item label="repeat_interval_seconds">
                  <el-input-number v-model="current.notify.repeat_interval_seconds" :min="0" :step="60" />
                </el-form-item>
                <el-form-item label="send_recovered"><el-switch v-model="current.notify.send_recovered" /></el-form-item>
              </el-form>
            </el-tab-pane>

            <el-tab-pane label="Daily" name="daily">
              <el-form label-width="180px" label-position="left">
                <el-form-item label="enabled"><el-switch v-model="current.daily_report.enabled" /></el-form-item>
                <el-form-item label="cron"><el-input v-model="current.daily_report.cron" placeholder="0 18 * * *" /></el-form-item>
                <el-form-item label="title_prefix"><el-input v-model="current.daily_report.title_prefix" /></el-form-item>
                <el-form-item label="max_lines"><el-input-number v-model="current.daily_report.max_lines" :min="1" :max="5000" /></el-form-item>
                <el-form-item label="max_chars"><el-input-number v-model="current.daily_report.max_chars" :min="100" :max="50000" /></el-form-item>
                <el-form-item label="clear_mode"><el-input v-model="current.daily_report.clear_mode" placeholder="reset_notified/clear_route/clear_all" /></el-form-item>
              </el-form>
            </el-tab-pane>

            <el-tab-pane label="Robots/Bindings JSON" name="rb">
              <el-alert
                title="体验版：此处先以 JSON 导入导出方式维护 robots/bindings（后端暂未提供读写 API）。"
                type="info"
                show-icon
                style="margin-bottom: 10px"
              />

              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px">
                <div style="font-weight: 600">robots</div>
                <div style="display: flex; gap: 8px">
                  <el-button size="small" @click="loadRbFromExport">从导出 JSON 填充</el-button>
                  <el-button size="small" type="primary" @click="applyRbToExport">写入导出 JSON</el-button>
                  <el-button size="small" type="success" @click="addRobot">新增 robot</el-button>
                </div>
              </div>

              <el-table :data="robotsTable" size="small" border style="width: 100%">
                <el-table-column label="id" width="140">
                  <template #default="scope">
                    <el-input v-model="scope.row.id" size="small" placeholder="robot id" />
                  </template>
                </el-table-column>
                <el-table-column label="webhook" min-width="240">
                  <template #default="scope">
                    <el-input v-model="scope.row.webhook" size="small" placeholder="https://oapi.dingtalk.com/robot/send?..." />
                  </template>
                </el-table-column>
                <el-table-column label="secret" min-width="180">
                  <template #default="scope">
                    <el-input v-model="scope.row.secret" size="small" placeholder="签名 secret（可空）" />
                  </template>
                </el-table-column>
                <el-table-column label="keyword" min-width="140">
                  <template #default="scope">
                    <el-input v-model="scope.row.keyword" size="small" placeholder="关键词（可空）" />
                  </template>
                </el-table-column>
                <el-table-column label="op" width="90">
                  <template #default="scope">
                    <el-button size="small" type="danger" @click="removeRobot(scope.$index)">删除</el-button>
                  </template>
                </el-table-column>
              </el-table>

              <el-divider />

              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px">
                <div style="font-weight: 600">bindings</div>
                <div style="display: flex; gap: 8px">
                  <el-button size="small" type="success" @click="addBinding">新增 binding</el-button>
                </div>
              </div>

              <el-table :data="bindingsTable" size="small" border style="width: 100%" height="360">
                <el-table-column label="enabled" width="90">
                  <template #default="scope">
                    <el-switch v-model="scope.row.enabled" />
                  </template>
                </el-table-column>
                <el-table-column label="name" width="160">
                  <template #default="scope">
                    <el-input v-model="scope.row.name" size="small" placeholder="binding name" />
                  </template>
                </el-table-column>
                <el-table-column label="priority" width="120">
                  <template #default="scope">
                    <el-input-number v-model="scope.row.priority" :min="0" :step="1" size="small" />
                  </template>
                </el-table-column>
                <el-table-column label="route_name" width="140">
                  <template #default="scope">
                    <el-input v-model="scope.row.route_name" size="small" placeholder="可空" />
                  </template>
                </el-table-column>
                <el-table-column label="group_id" width="120">
                  <template #default="scope">
                    <el-input-number v-model="scope.row.group_id" :min="0" :step="1" size="small" />
                  </template>
                </el-table-column>
                <el-table-column label="rule_id" width="120">
                  <template #default="scope">
                    <el-input-number v-model="scope.row.rule_id" :min="0" :step="1" size="small" />
                  </template>
                </el-table-column>
                <el-table-column label="group_name_regex" min-width="180">
                  <template #default="scope">
                    <el-input v-model="scope.row.group_name_regex" size="small" placeholder="可空" />
                  </template>
                </el-table-column>
                <el-table-column label="rule_name_regex" min-width="180">
                  <template #default="scope">
                    <el-input v-model="scope.row.rule_name_regex" size="small" placeholder="可空" />
                  </template>
                </el-table-column>
                <el-table-column label="robot_ids" min-width="220">
                  <template #default="scope">
                    <el-select
                      v-model="scope.row.robot_ids"
                      multiple
                      filterable
                      allow-create
                      default-first-option
                      size="small"
                      placeholder="选择/输入 robot id"
                      style="width: 100%"
                    >
                      <el-option v-for="rb in robotsTable" :key="rb.id" :label="rb.id" :value="rb.id" />
                    </el-select>
                  </template>
                </el-table-column>
                <el-table-column label="tags" min-width="160">
                  <template #default="scope">
                    <div style="display: flex; gap: 8px; align-items: center">
                      <el-tag size="small" type="info">{{ (scope.row.tags_kv || []).length }}</el-tag>
                      <el-button size="small" @click="openBindingKvEditor(scope.$index, 'tags')">编辑</el-button>
                    </div>
                  </template>
                </el-table-column>
                <el-table-column label="tag_regex" min-width="160">
                  <template #default="scope">
                    <div style="display: flex; gap: 8px; align-items: center">
                      <el-tag size="small" type="info">{{ (scope.row.tag_regex_kv || []).length }}</el-tag>
                      <el-button size="small" @click="openBindingKvEditor(scope.$index, 'tag_regex')">编辑</el-button>
                    </div>
                  </template>
                </el-table-column>
                <el-table-column label="op" width="90">
                  <template #default="scope">
                    <el-button size="small" type="danger" @click="removeBinding(scope.$index)">删除</el-button>
                  </template>
                </el-table-column>
              </el-table>

              <el-divider />

              <el-collapse>
                <el-collapse-item title="高级：JSON（可手工编辑）" name="json">
                  <el-row :gutter="12">
                    <el-col :span="12">
                      <div style="font-weight: 600; margin-bottom: 6px">robots</div>
                      <el-input v-model="robotsJson" type="textarea" :rows="12" placeholder="[{id,webhook,secret,keyword}]" />
                      <div style="margin-top: 8px">
                        <el-button size="small" @click="syncRobotsTableFromJson">JSON → 表格</el-button>
                        <el-button size="small" @click="syncRobotsJsonFromTable">表格 → JSON</el-button>
                      </div>
                    </el-col>
                    <el-col :span="12">
                      <div style="font-weight: 600; margin-bottom: 6px">bindings</div>
                      <el-input v-model="bindingsJson" type="textarea" :rows="12" placeholder="[{name,priority,enabled,robot_id/robot_ids,...}]" />
                      <div style="margin-top: 8px">
                        <el-button size="small" @click="syncBindingsTableFromJson">JSON → 表格</el-button>
                        <el-button size="small" @click="syncBindingsJsonFromTable">表格 → JSON</el-button>
                      </div>
                    </el-col>
                  </el-row>
                </el-collapse-item>
              </el-collapse>
            </el-tab-pane>
          </el-tabs>
        </div>
      </el-card>
    </el-col>
  </el-row>

  <el-dialog v-model="importDialog" title="导入 JSON" width="70%">
    <el-alert
      title="导入将覆盖当前页面内的 routes/robots/bindings 本地副本（不会写入后端）。"
      type="warning"
      show-icon
      style="margin-bottom: 10px"
    />
    <el-input v-model="importText" type="textarea" :rows="16" placeholder="粘贴 JSON：{ routes:[], robots:[], bindings:[] } 或 { routes:[] }" />
    <template #footer>
      <el-button @click="importDialog = false">取消</el-button>
      <el-button type="primary" @click="doImport">导入</el-button>
    </template>
  </el-dialog>

  <el-drawer v-model="previewDrawer" title="样本预览（基于当前服务配置）" size="70%">
    <el-alert
      title="注意：预览基于服务当前生效配置；你在本页编辑的内容需要导出写回配置并重启后才能真正影响预览结果。"
      type="warning"
      show-icon
      style="margin-bottom: 10px"
    />

    <el-row :gutter="12">
      <el-col :span="10">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
          <div>CurEvent JSON</div>
          <el-button type="primary" :loading="previewLoading" @click="doPreview">预览</el-button>
        </div>
        <el-input v-model="previewText" type="textarea" :rows="18" placeholder="单条或数组 CurEvent" />
        <div style="margin-top: 10px; display: flex; gap: 8px">
          <el-button size="small" @click="loadSampleFromStorage">从告警样本填充</el-button>
          <el-button size="small" @click="clearSampleStorage">清空样本</el-button>
        </div>
      </el-col>
      <el-col :span="14">
        <el-alert v-if="previewError" :title="previewError" type="error" show-icon style="margin-bottom: 10px" />
        <el-table :data="previewItems" size="small" border height="420">
          <el-table-column prop="route_name" label="Route" width="120" />
          <el-table-column prop="dedup_key" label="DedupKey" min-width="220" />
          <el-table-column prop="final_robot_ids" label="Robots" min-width="180">
            <template #default="scope">
              <el-tag v-for="rid in (scope.row.final_robot_ids || [])" :key="rid" size="small" style="margin-right: 6px">{{ rid }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
        <div style="margin-top: 10px">
          <el-collapse>
            <el-collapse-item v-for="(it, idx) in previewItems" :key="idx" :title="it.service_hash || String(idx)">
              <pre style="margin: 0; white-space: pre-wrap">{{ JSON.stringify(it, null, 2) }}</pre>
            </el-collapse-item>
          </el-collapse>
        </div>
      </el-col>
    </el-row>
  </el-drawer>

  <el-dialog v-model="bindingKvDialog" :title="bindingKvDialogTitle" width="60%">
    <el-alert
      title="Key-Value 编辑：key 为空会被忽略；tag_regex 的 value 会当作正则表达式参与匹配。"
      type="info"
      show-icon
      style="margin-bottom: 10px"
    />

    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px">
      <div style="font-weight: 600">{{ bindingKvEditorType === 'tags' ? 'tags' : 'tag_regex' }}</div>
      <el-button size="small" type="success" @click="addKvRow">新增行</el-button>
    </div>

    <el-table :data="bindingKvDraft" size="small" border style="width: 100%" height="360">
      <el-table-column label="key" min-width="220">
        <template #default="scope">
          <el-input v-model="scope.row.key" size="small" placeholder="例如 cluster" />
        </template>
      </el-table-column>
      <el-table-column :label="bindingKvEditorType === 'tags' ? 'value' : 'regex'" min-width="320">
        <template #default="scope">
          <el-input v-model="scope.row.value" size="small" placeholder="例如 prod 或 prod-.*" />
        </template>
      </el-table-column>
      <el-table-column label="op" width="90">
        <template #default="scope">
          <el-button size="small" type="danger" @click="removeKvRow(scope.$index)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <template #footer>
      <el-button @click="bindingKvDialog = false">取消</el-button>
      <el-button type="primary" @click="confirmKvEditor">确定</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRoute, useRouter } from 'vue-router'

import { getRoutes, previewRoutes } from '../api/client'
import type { RouteItem } from '../api/types'

const loading = ref(false)
const error = ref('')

const route = useRoute()
const router = useRouter()

const routes = ref([] as RouteItem[])
const activeName = ref('')
const activeTab = ref('match')
const filterText = ref('')

const exportObj = ref({ routes: [], robots: [], bindings: [] } as any)
const robotsJson = ref('[]')
const bindingsJson = ref('[]')

type RobotRow = {
  id: string
  webhook: string
  secret?: string
  keyword?: string
}

function kvToObj(kv: Array<{ key: string; value: string }>): any {
  const out: any = {}
  for (const it of kv || []) {
    const k = String(it?.key || '').trim()
    const v = String(it?.value || '')
    if (!k) continue
    out[k] = v
  }
  return out
}

type BindingRow = {
  // 中文说明：tags/tag_regex 使用 KV 表格编辑，最终导出会转换为对象
  name: string
  enabled: boolean
  priority: number
  route_name?: string
  group_id?: number
  group_name_regex?: string
  rule_id?: number
  rule_name_regex?: string
  robot_ids: string[]
  tags_kv: Array<{ key: string; value: string }>
  tag_regex_kv: Array<{ key: string; value: string }>
  // 兜底：高级 JSON 编辑（不在表格内直接编辑）
  tags_json: string
  tag_regex_json: string
}

const robotsTable = ref([] as RobotRow[])
const bindingsTable = ref([] as BindingRow[])

const bindingKvDialog = ref(false)
const bindingKvEditorIndex = ref(-1)
const bindingKvEditorType = ref('tags' as 'tags' | 'tag_regex')
const bindingKvDraft = ref([] as Array<{ key: string; value: string }>)

const bindingKvDialogTitle = computed(() => {
  const idx = bindingKvEditorIndex.value
  const name = idx >= 0 && idx < bindingsTable.value.length ? String(bindingsTable.value[idx]?.name || '') : ''
  const t = bindingKvEditorType.value === 'tags' ? 'tags' : 'tag_regex'
  return name ? `编辑 ${t} - ${name}` : `编辑 ${t}`
})

const importDialog = ref(false)
const importText = ref('')

const previewDrawer = ref(false)
const previewText = ref('')
const previewLoading = ref(false)
const previewError = ref('')
const previewItems = ref([] as any[])

const SAMPLE_KEY = 'n9e_alter_preview_sample'

const filteredRoutes = computed(() => {
  const kw = filterText.value.trim().toLowerCase()
  if (!kw) return routes.value
  return routes.value.filter((r: RouteItem) => String(r.name || '').toLowerCase().includes(kw))
})

function safeCompileRegex(pat: string): string {
  const s = String(pat || '').trim()
  if (!s) return ''
  try {
    // eslint-disable-next-line no-new
    new RegExp(s)
    return ''
  } catch (e: any) {
    return e?.message || 'invalid regex'
  }
}

function handleAutoPreviewFromQuery() {
  const q: any = route.query || {}
  const preview = String(q.preview || '').trim()
  if (preview !== '1') return

  const wantRoute = String(q.route || '').trim()
  if (wantRoute) {
    const found = routes.value.find((r: RouteItem) => String(r.name || '') === wantRoute)
    if (found) activeName.value = found.name
  }

  openPreview()

  // 自动触发一次预览（确保 openPreview 已填充 previewText）
  setTimeout(() => {
    if (!previewDrawer.value) return
    if (!String(previewText.value || '').trim()) return
    // eslint-disable-next-line @typescript-eslint/no-floating-promises
    doPreview()
  }, 0)

  // 清理 query，避免刷新/热更新重复触发
  const nq: any = { ...q }
  delete nq.preview
  delete nq.route
  router.replace({ path: route.path, query: nq })
}

const validationErrors = computed(() => {
  const errs: string[] = []

  // routes 基础校验
  const nameSet = new Set<string>()
  for (const r of routes.value) {
    const name = String((r as any).name || '').trim()
    if (!name) {
      errs.push('routes: 存在 name 为空的规则')
      continue
    }
    if (nameSet.has(name)) {
      errs.push(`routes: name 重复: ${name}`)
      continue
    }
    nameSet.add(name)

    const g = String((r as any).match?.group_name_regex || '').trim()
    const ru = String((r as any).match?.rule_name_regex || '').trim()
    if (!g) errs.push(`route=${name}: match.group_name_regex 必填`)
    if (!ru) errs.push(`route=${name}: match.rule_name_regex 必填`)
    const eg = safeCompileRegex(g)
    if (eg) errs.push(`route=${name}: group_name_regex regex错误: ${eg}`)
    const er = safeCompileRegex(ru)
    if (er) errs.push(`route=${name}: rule_name_regex regex错误: ${er}`)

    const rws = ((r as any).dedup?.rewrites || []) as any[]
    for (let i = 0; i < rws.length; i++) {
      const rw = rws[i]
      const f = String(rw?.field || '').trim()
      const p = String(rw?.pattern || '').trim()
      if (!f) errs.push(`route=${name}: rewrites[${i}].field 必填`)
      if (!p) errs.push(`route=${name}: rewrites[${i}].pattern 必填`)
      const ep = safeCompileRegex(p)
      if (ep) errs.push(`route=${name}: rewrites[${i}].pattern regex错误: ${ep}`)
    }
  }

  // bindings 校验：基于表格数据
  for (let i = 0; i < bindingsTable.value.length; i++) {
    const b = bindingsTable.value[i]
    if (b && b.enabled === false) continue
    const name = String(b.name || '').trim()
    if (!name) errs.push(`bindings[${i}]: name 必填`)
    const pr = Number(b.priority)
    if (!Number.isFinite(pr)) errs.push(`bindings[${i}]: priority 必填且必须为数字`)
    const rids = Array.isArray(b.robot_ids) ? b.robot_ids.filter((x: any) => String(x || '').trim() !== '') : []
    if (rids.length === 0) errs.push(`bindings[${i}]: robot_ids 至少填一个`)

    const g = String(b.group_name_regex || '').trim()
    const ru = String(b.rule_name_regex || '').trim()
    const eg = safeCompileRegex(g)
    if (eg && g) errs.push(`bindings[${i}]: group_name_regex regex错误: ${eg}`)
    const er = safeCompileRegex(ru)
    if (er && ru) errs.push(`bindings[${i}]: rule_name_regex regex错误: ${er}`)

    const kv1 = Array.isArray(b.tags_kv) ? b.tags_kv : []
    for (let j = 0; j < kv1.length; j++) {
      const k = String(kv1[j]?.key || '').trim()
      if (!k && String(kv1[j]?.value || '').trim()) errs.push(`bindings[${i}]: tags[${j}] key 不能为空`)
    }

    const kv2 = Array.isArray(b.tag_regex_kv) ? b.tag_regex_kv : []
    for (let j = 0; j < kv2.length; j++) {
      const k = String(kv2[j]?.key || '').trim()
      const v = String(kv2[j]?.value || '').trim()
      if (!k && v) errs.push(`bindings[${i}]: tag_regex[${j}] key 不能为空`)
      if (v) {
        const ep = safeCompileRegex(v)
        if (ep) errs.push(`bindings[${i}]: tag_regex[${k || String(j)}] regex错误: ${ep}`)
      }
    }
  }

  // robots 校验：基于表格数据
  const robotIdSet = new Set<string>()
  for (let i = 0; i < robotsTable.value.length; i++) {
    const rb = robotsTable.value[i]
    const id = String(rb.id || '').trim()
    const w = String(rb.webhook || '').trim()
    if (!id) errs.push(`robots[${i}]: id 必填`)
    if (!w) errs.push(`robots[${i}]: webhook 必填`)
    if (id) {
      if (robotIdSet.has(id)) errs.push(`robots: id 重复: ${id}`)
      robotIdSet.add(id)
    }
  }

  return errs
})

const validationWarnings = computed(() => {
  const warns: string[] = []
  // bindings priority 冲突：同 priority 且匹配条件完全一致
  const m = new Map<string, number[]>()
  for (let i = 0; i < bindingsTable.value.length; i++) {
    const b = bindingsTable.value[i]
    if (b && b.enabled === false) continue
    const pr = Number(b.priority)
    if (!Number.isFinite(pr)) continue
    const keyObj: any = {
      route_name: b.route_name || '',
      group_id: b.group_id || 0,
      group_name_regex: b.group_name_regex || '',
      rule_id: b.rule_id || 0,
      rule_name_regex: b.rule_name_regex || '',
      tags_json: b.tags_json || '',
      tag_regex_json: b.tag_regex_json || '',
    }
    const key = `${pr}|${JSON.stringify(keyObj)}`
    const xs = m.get(key) || []
    xs.push(i)
    m.set(key, xs)
  }
  for (const [k, idxs] of m.entries()) {
    if (idxs.length > 1) {
      const pr = k.split('|')[0]
      warns.push(`bindings: 存在 priority 冲突（同 priority 且匹配条件一致），priority=${pr} indexes=${idxs.join(',')}`)
    }
  }
  return warns
})

const current = computed(() => {
  if (!activeName.value) return null
  return routes.value.find((r: RouteItem) => r.name === activeName.value) || null
})

const severityIn = computed({
  get() {
    const c: any = current.value
    if (!c) return [] as number[]
    return (c.match?.severity_in || []) as number[]
  },
  set(v: number[]) {
    const c: any = current.value
    if (!c) return
    c.match.severity_in = v
  },
})

function onSelect(name: string) {
  activeName.value = name
}

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    const data = await getRoutes()
    routes.value = (data.items || []).map((r) => JSON.parse(JSON.stringify(r)))
    exportObj.value.routes = JSON.parse(JSON.stringify(routes.value))
    if (!activeName.value && routes.value.length > 0) activeName.value = routes.value[0].name
  } catch (e: any) {
    error.value = e?.message || String(e)
    ElMessage.error(`routes: ${error.value}`)
  } finally {
    loading.value = false
  }
}

function addRoute() {
  const name = `route-${Date.now()}`
  const it: any = {
    name,
    enabled: true,
    match: { group_name_regex: '.*', rule_name_regex: '.*', severity_in: [] },
    dedup: {
      mode: 'n9e_hash',
      include_group_id: true,
      include_group_name: true,
      include_rule_id: true,
      include_rule_name: true,
      include_severity: true,
      include_entity: true,
      normalize_pod_name: false,
      rewrites: [],
    },
    notify: { enabled: false, observe_seconds: 0, repeat_interval_seconds: 3600, send_recovered: true, robot_id: '' },
    daily_report: { enabled: false, cron: '0 18 * * *', title_prefix: 'N9E 告警日报', max_lines: 50, max_chars: 15000, clear_mode: 'reset_notified' },
  }
  routes.value = [it as RouteItem, ...routes.value]
  activeName.value = name
  exportObj.value.routes = JSON.parse(JSON.stringify(routes.value))
}

function cloneRoute() {
  if (!current.value) return
  const c: any = JSON.parse(JSON.stringify(current.value))
  c.name = `${c.name}-copy-${Date.now()}`
  routes.value = [c as RouteItem, ...routes.value]
  activeName.value = c.name
  exportObj.value.routes = JSON.parse(JSON.stringify(routes.value))
}

async function removeRoute() {
  if (!current.value) return
  const name = current.value.name
  const ok = await ElMessageBox.confirm(`确认删除 route: ${name} ?`, '确认', { type: 'warning' }).catch(() => '')
  if (!ok) return
  routes.value = routes.value.filter((r: RouteItem) => r.name !== name)
  activeName.value = routes.value.length > 0 ? routes.value[0].name : ''
  exportObj.value.routes = JSON.parse(JSON.stringify(routes.value))
}

function ensureRewritesArr() {
  const c: any = current.value
  if (!c) return
  if (!c.dedup.rewrites) c.dedup.rewrites = []
}

function addRewrite() {
  ensureRewritesArr()
  const c: any = current.value
  c.dedup.rewrites.push({ field: 'group_name', pattern: '', replace: '' })
}

function removeRewrite(idx: number) {
  ensureRewritesArr()
  const c: any = current.value
  c.dedup.rewrites.splice(idx, 1)
}

function exportJson() {
  exportObj.value.routes = JSON.parse(JSON.stringify(routes.value))
  exportObj.value.robots = buildRobotsForExport()
  exportObj.value.bindings = buildBindingsForExport()
  const txt = JSON.stringify(exportObj.value, null, 2)
  navigator.clipboard
    .writeText(txt)
    .then(() => ElMessage.success('已复制到剪贴板'))
    .catch(() => ElMessage.info('复制失败，请手动复制'))
}

function openImport() {
  importText.value = ''
  importDialog.value = true
}

function doImport() {
  let obj: any
  try {
    obj = JSON.parse(importText.value)
  } catch (e: any) {
    ElMessage.error(e?.message || 'JSON 解析失败')
    return
  }

  const rts = obj.routes || obj.items || obj
  if (Array.isArray(rts)) {
    routes.value = rts
    exportObj.value.routes = JSON.parse(JSON.stringify(routes.value))
    if (routes.value.length > 0) activeName.value = String(routes.value[0].name || '')
  }

  if (obj.robots) robotsJson.value = JSON.stringify(obj.robots, null, 2)
  if (obj.bindings) bindingsJson.value = JSON.stringify(obj.bindings, null, 2)
  syncRobotsTableFromJson()
  syncBindingsTableFromJson()

  importDialog.value = false
  ElMessage.success('导入完成（仅本地副本）')
}

function loadRbFromExport() {
  robotsJson.value = JSON.stringify(exportObj.value.robots || [], null, 2)
  bindingsJson.value = JSON.stringify(exportObj.value.bindings || [], null, 2)
  syncRobotsTableFromJson()
  syncBindingsTableFromJson()
}

function applyRbToExport() {
  exportObj.value.robots = buildRobotsForExport()
  exportObj.value.bindings = buildBindingsForExport()
  ElMessage.success('已写入导出 JSON（仍需导出后落盘）')
}

function addRobot() {
  robotsTable.value.push({ id: '', webhook: '', secret: '', keyword: '' })
  syncRobotsJsonFromTable()
}

function removeRobot(idx: number) {
  robotsTable.value.splice(idx, 1)
  syncRobotsJsonFromTable()
}

function addBinding() {
  bindingsTable.value.push({
    name: '',
    enabled: true,
    priority: 100,
    route_name: '',
    group_id: 0,
    group_name_regex: '',
    rule_id: 0,
    rule_name_regex: '',
    robot_ids: [],
    tags_kv: [],
    tag_regex_kv: [],
    tags_json: '{}',
    tag_regex_json: '{}',
  })
  syncBindingsJsonFromTable()
}

function removeBinding(idx: number) {
  bindingsTable.value.splice(idx, 1)
  syncBindingsJsonFromTable()
}

function buildRobotsForExport(): any[] {
  return robotsTable.value
    .map((r: RobotRow) => ({
      id: String(r.id || '').trim(),
      webhook: String(r.webhook || '').trim(),
      secret: String(r.secret || ''),
      keyword: String(r.keyword || ''),
    }))
    .filter((r: any) => r.id || r.webhook)
}

function parseJsonObjectOrEmpty(s: string): any {
  const txt = String(s || '').trim()
  if (!txt) return {}
  const obj = JSON.parse(txt)
  if (!obj || typeof obj !== 'object' || Array.isArray(obj)) return {}
  return obj
}

function buildBindingsForExport(): any[] {
  return bindingsTable.value
    .map((b: BindingRow) => {
      const tagsObj = kvToObj(b.tags_kv || [])
      const tagRegexObj = kvToObj(b.tag_regex_kv || [])
      const obj: any = {
        name: String(b.name || '').trim(),
        enabled: b.enabled !== false,
        priority: Number(b.priority),
        route_name: String(b.route_name || '').trim(),
        group_id: Number(b.group_id || 0),
        group_name_regex: String(b.group_name_regex || '').trim(),
        rule_id: Number(b.rule_id || 0),
        rule_name_regex: String(b.rule_name_regex || '').trim(),
        robot_ids: Array.isArray(b.robot_ids) ? b.robot_ids.filter((x: any) => String(x || '').trim() !== '') : [],
        tags: tagsObj,
        tag_regex: tagRegexObj,
      }
      if (!obj.route_name) delete obj.route_name
      if (!obj.group_id) delete obj.group_id
      if (!obj.group_name_regex) delete obj.group_name_regex
      if (!obj.rule_id) delete obj.rule_id
      if (!obj.rule_name_regex) delete obj.rule_name_regex
      if (!obj.tags || Object.keys(obj.tags).length === 0) delete obj.tags
      if (!obj.tag_regex || Object.keys(obj.tag_regex).length === 0) delete obj.tag_regex
      return obj
    })
    .filter((b: any) => b.name || (Array.isArray(b.robot_ids) && b.robot_ids.length > 0))
}

function syncRobotsTableFromJson() {
  try {
    const arr = JSON.parse(robotsJson.value || '[]')
    if (!Array.isArray(arr)) return
    robotsTable.value = arr.map((x: any) => ({
      id: String(x?.id || ''),
      webhook: String(x?.webhook || ''),
      secret: String(x?.secret || ''),
      keyword: String(x?.keyword || ''),
    }))
  } catch {
    // ignore
  }
}

function syncBindingsTableFromJson() {
  try {
    const arr = JSON.parse(bindingsJson.value || '[]')
    if (!Array.isArray(arr)) return
    bindingsTable.value = arr.map((x: any) => {
      const robotIds = Array.isArray(x?.robot_ids) ? x.robot_ids.map((t: any) => String(t || '')).filter((t: string) => t.trim() !== '') : []
      const rid = String(x?.robot_id || '').trim()
      const mergedIds = [...robotIds]
      if (rid && !mergedIds.includes(rid)) mergedIds.push(rid)

      const tagsObj = x?.tags && typeof x.tags === 'object' && !Array.isArray(x.tags) ? x.tags : {}
      const tagRegexObj = x?.tag_regex && typeof x.tag_regex === 'object' && !Array.isArray(x.tag_regex) ? x.tag_regex : {}
      const tagsKv = Object.keys(tagsObj || {}).map((k: string) => ({ key: k, value: String((tagsObj as any)[k] ?? '') }))
      const tagRegexKv = Object.keys(tagRegexObj || {}).map((k: string) => ({ key: k, value: String((tagRegexObj as any)[k] ?? '') }))

      return {
        name: String(x?.name || ''),
        enabled: x?.enabled !== false,
        priority: Number(x?.priority ?? 0),
        route_name: String(x?.route_name || ''),
        group_id: Number(x?.group_id ?? 0),
        group_name_regex: String(x?.group_name_regex || ''),
        rule_id: Number(x?.rule_id ?? 0),
        rule_name_regex: String(x?.rule_name_regex || ''),
        robot_ids: mergedIds,
        tags_kv: tagsKv,
        tag_regex_kv: tagRegexKv,
        tags_json: JSON.stringify(tagsObj || {}, null, 2),
        tag_regex_json: JSON.stringify(tagRegexObj || {}, null, 2),
      } as BindingRow
    })
  } catch {
    // ignore
  }
}

function syncRobotsJsonFromTable() {
  robotsJson.value = JSON.stringify(buildRobotsForExport(), null, 2)
}

function syncBindingsJsonFromTable() {
  bindingsJson.value = JSON.stringify(buildBindingsForExport(), null, 2)
}

function openBindingKvEditor(idx: number, t: 'tags' | 'tag_regex') {
  bindingKvEditorIndex.value = idx
  bindingKvEditorType.value = t
  const row = bindingsTable.value[idx]
  const src = t === 'tags' ? row.tags_kv : row.tag_regex_kv
  bindingKvDraft.value = (src || []).map((it: { key: string; value: string }) => ({
    key: String(it?.key || ''),
    value: String(it?.value || ''),
  }))
  bindingKvDialog.value = true
}

function addKvRow() {
  bindingKvDraft.value.push({ key: '', value: '' })
}

function removeKvRow(i: number) {
  bindingKvDraft.value.splice(i, 1)
}

function confirmKvEditor() {
  const idx = bindingKvEditorIndex.value
  if (idx < 0 || idx >= bindingsTable.value.length) {
    bindingKvDialog.value = false
    return
  }
  const cleaned = (bindingKvDraft.value || []).map((it: { key: string; value: string }) => ({
    key: String(it?.key || ''),
    value: String(it?.value || ''),
  }))
  const row = bindingsTable.value[idx]
  if (bindingKvEditorType.value === 'tags') {
    row.tags_kv = cleaned
    row.tags_json = JSON.stringify(kvToObj(cleaned), null, 2)
  } else {
    row.tag_regex_kv = cleaned
    row.tag_regex_json = JSON.stringify(kvToObj(cleaned), null, 2)
  }
  bindingKvDialog.value = false
}

watch(
  robotsTable,
  () => {
    syncRobotsJsonFromTable()
  },
  { deep: true },
)

watch(
  bindingsTable,
  () => {
    syncBindingsJsonFromTable()
  },
  { deep: true },
)

function openPreview() {
  previewDrawer.value = true
  previewError.value = ''
  previewItems.value = []
  if (!previewText.value) {
    loadSampleFromStorage()
  }
  if (!previewText.value) {
    previewText.value = JSON.stringify(
      {
        id: 1,
        hash: 'xxx',
        group_id: 100,
        group_name: 'demo-group',
        rule_id: 200,
        rule_name: 'demo-rule',
        severity: 2,
        first_trigger_time: Math.floor(Date.now() / 1000) - 60,
        trigger_time: Math.floor(Date.now() / 1000),
        tags: { cluster: 'prod', namespace: 'default' },
      },
      null,
      2,
    )
  }
}

function loadSampleFromStorage() {
  try {
    const s = localStorage.getItem(SAMPLE_KEY)
    if (!s) return
    const obj = JSON.parse(s)
    previewText.value = JSON.stringify(obj, null, 2)
    ElMessage.success('已从告警样本填充')
  } catch {
    // ignore
  }
}

function clearSampleStorage() {
  try {
    localStorage.removeItem(SAMPLE_KEY)
    ElMessage.success('已清空样本')
  } catch {
    // ignore
  }
}

async function doPreview() {
  previewError.value = ''
  previewItems.value = []

  let payload: any
  try {
    payload = JSON.parse(previewText.value)
  } catch (e: any) {
    previewError.value = e?.message || 'JSON 解析失败'
    return
  }

  previewLoading.value = true
  try {
    const resp = await previewRoutes(payload)
    previewItems.value = resp.items || []
  } catch (e: any) {
    previewError.value = e?.message || String(e)
  } finally {
    previewLoading.value = false
  }
}

onMounted(async () => {
  await refresh()
  syncRobotsTableFromJson()
  syncBindingsTableFromJson()
  handleAutoPreviewFromQuery()
})
</script>
