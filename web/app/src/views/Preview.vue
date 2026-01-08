<template>
  <el-row :gutter="12">
    <el-col :span="10">
      <el-card>
        <template #header>
          <div style="display: flex; justify-content: space-between; align-items: center">
            <div>预览输入（CurEvent JSON：单条或数组）</div>
            <el-button type="primary" :loading="loading" @click="doPreview">预览</el-button>
          </div>
        </template>

        <el-input
          v-model="jsonText"
          type="textarea"
          :rows="22"
          placeholder="粘贴 n9e CurEvent JSON，支持单条对象或数组"
        />

        <div style="margin-top: 10px; display: flex; gap: 10px; align-items: center">
          <el-button @click="loadSample">加载示例</el-button>
          <el-text type="info">后端接口：POST /api/v1/routes/preview</el-text>
        </div>
      </el-card>
    </el-col>

    <el-col :span="14">
      <el-card>
        <template #header>
          <div style="display: flex; justify-content: space-between; align-items: center">
            <div>预览结果</div>
            <div style="display: flex; gap: 8px; align-items: center">
              <el-input v-model="filterText" placeholder="搜索 route/group/rule/entity/tag" style="width: 260px" clearable />
              <el-button :disabled="!selectedRow" type="primary" @click="openGen">从选中事件生成 Binding</el-button>
            </div>
          </div>
        </template>

        <el-alert v-if="error" :title="error" type="error" show-icon style="margin-bottom: 10px" />

        <el-table :data="filteredItems" size="small" border style="width: 100%" height="520" highlight-current-row @current-change="onCurrentChange">
          <el-table-column type="index" width="48" />
          <el-table-column prop="route_name" label="Route" width="130" />
          <el-table-column prop="group_id" label="GroupID" width="90" />
          <el-table-column prop="rule_id" label="RuleID" width="90" />
          <el-table-column prop="severity" label="Sev" width="60" />
          <el-table-column prop="dedup_key" label="DedupKey" min-width="220" />
          <el-table-column prop="final_robot_ids" label="Robots" min-width="180">
            <template #default="scope">
              <el-tag v-for="rid in (scope.row.final_robot_ids || [])" :key="rid" style="margin-right: 6px" size="small">
                {{ rid }}
              </el-tag>
            </template>
          </el-table-column>
        </el-table>

        <el-divider />

        <el-collapse v-model="activeNames">
          <el-collapse-item v-for="(it, idx) in filteredItems" :key="idx" :name="String(idx)">
            <template #title>
              <div style="display: flex; gap: 10px; align-items: center">
                <el-tag size="small">{{ it.route_name }}</el-tag>
                <span style="font-size: 12px; color: #666">{{ it.service_hash }}</span>
              </div>
            </template>

            <el-descriptions :column="1" size="small" border>
              <el-descriptions-item label="group_name">{{ it.group_name_before }} → {{ it.group_name_after }}</el-descriptions-item>
              <el-descriptions-item label="rule_name">{{ it.rule_name_before }} → {{ it.rule_name_after }}</el-descriptions-item>
              <el-descriptions-item label="entity">{{ it.entity_before }} → {{ it.entity_after }}</el-descriptions-item>
              <el-descriptions-item label="matched_bindings">
                <el-tag v-for="b in (it.matched_bindings || [])" :key="b.name" type="info" size="small" style="margin-right: 6px">
                  {{ b.name }} (p={{ b.priority }})
                </el-tag>
              </el-descriptions-item>
              <el-descriptions-item label="tags">
                <pre style="margin: 0; white-space: pre-wrap">{{ JSON.stringify(it.tags || {}, null, 2) }}</pre>
              </el-descriptions-item>
            </el-descriptions>
          </el-collapse-item>
        </el-collapse>
      </el-card>
    </el-col>
  </el-row>

  <el-dialog v-model="gen.open" title="生成 Binding（tag/tag_regex 分流）" width="860px">
    <el-alert
      v-if="gen.err"
      type="error"
      :closable="false"
      show-icon
      :title="gen.err"
      style="margin-bottom: 10px"
    />

    <el-form label-width="140px">
      <el-form-item label="binding.name">
        <el-input v-model="gen.name" placeholder="例如: env_prod_to_groupA" />
      </el-form-item>
      <el-form-item label="priority">
        <el-input-number v-model="gen.priority" :step="1" />
      </el-form-item>
      <el-form-item label="route_name">
        <el-input v-model="gen.route" placeholder="可选，留空=所有 route" />
      </el-form-item>
      <el-form-item label="robot_ids">
        <el-input v-model="gen.robotIDsText" placeholder="逗号分隔，例如: env_prod_ops,env_prod_bak" />
      </el-form-item>
      <el-form-item label="生成字段">
        <el-radio-group v-model="gen.field">
          <el-radio-button label="tag_regex">tag_regex（正则）</el-radio-button>
          <el-radio-button label="tags">tags（精确）</el-radio-button>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="keys 搜索">
        <el-input v-model="gen.keyFilter" placeholder="搜索 key" clearable />
      </el-form-item>
      <el-form-item label="选择 keys">
        <div style="display: flex; gap: 8px; margin-bottom: 8px">
          <el-button size="small" @click="genSelectAll">全选</el-button>
          <el-button size="small" @click="genSelectNone">全不选</el-button>
          <el-button size="small" @click="genSelectCommon">仅 cluster/app/env</el-button>
        </div>
        <el-checkbox-group v-model="gen.selectedKeys" style="display: flex; flex-wrap: wrap; gap: 10px">
          <el-checkbox v-for="k in genFilteredKeys" :key="k" :label="k">{{ k }}</el-checkbox>
        </el-checkbox-group>
      </el-form-item>
      <el-form-item label="预览生成结果">
        <el-input v-model="gen.previewJson" type="textarea" :autosize="{ minRows: 8, maxRows: 16 }" readonly />
      </el-form-item>
    </el-form>

    <template #footer>
      <div style="display: flex; justify-content: space-between; width: 100%">
        <div style="display: flex; gap: 8px">
          <el-button @click="buildPreview">刷新预览</el-button>
        </div>
        <div style="display: flex; gap: 8px">
          <el-button @click="gen.open = false">取消</el-button>
          <el-button type="primary" @click="saveAndGotoSettings">保存并跳转 Settings</el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useRouter } from 'vue-router'

import { previewRoutes } from '../api/client'
import type { PreviewItem } from '../api/types'

const jsonText = ref('')
const loading = ref(false)
const error = ref('')
const items = ref([] as PreviewItem[])
const activeNames = ref([] as string[])

const filterText = ref('')
const selectedRow = ref<any | null>(null)

const router = useRouter()

const SAMPLE_KEY = 'n9e_alter_preview_sample'
const BINDING_DRAFT_KEY = 'n9e_alter_binding_draft'

function loadSampleFromStorage() {
  try {
    const s = localStorage.getItem(SAMPLE_KEY)
    if (!s) return
    const obj = JSON.parse(s)
    jsonText.value = JSON.stringify(obj, null, 2)
  } catch {
    // ignore
  }
}

function loadSample() {
  jsonText.value = JSON.stringify(
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
      tags: {
        cluster: 'prod',
        namespace: 'default',
        pod: 'demo-pod-1',
      },
    },
    null,
    2,
  )
}

async function doPreview() {
  error.value = ''
  items.value = []
  activeNames.value = []

  let payload: any
  try {
    payload = JSON.parse(jsonText.value)
  } catch (e: any) {
    error.value = e?.message || 'JSON 解析失败'
    return
  }

  loading.value = true
  try {
    const resp = await previewRoutes(payload)
    items.value = resp.items || []
    try {
      // 记录最近一次成功预览的样本，便于后续快速加载
      localStorage.setItem(SAMPLE_KEY, JSON.stringify(payload))
    } catch {
      // ignore persistence error
    }
    activeNames.value = items.value.slice(0, 3).map((_: any, i: number) => String(i))
    selectedRow.value = null
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    loading.value = false
  }
}

const filteredItems = computed(() => {
  const kw = String(filterText.value || '').trim().toLowerCase()
  if (!kw) return items.value
  const out: any[] = []
  for (const it of items.value || []) {
    const hay = [
      it?.route_name,
      it?.group_name_after,
      it?.rule_name_after,
      it?.entity_after,
      JSON.stringify(it?.tags || {}),
    ]
      .filter(Boolean)
      .join(' ')
      .toLowerCase()
    if (hay.includes(kw)) out.push(it)
  }
  return out
})

function onCurrentChange(row: any) {
  selectedRow.value = row || null
}

const gen = ref({
  open: false,
  err: '',
  field: 'tag_regex' as 'tags' | 'tag_regex',
  name: '',
  priority: 200,
  route: '',
  robotIDsText: '',
  keyFilter: '',
  keys: [] as string[],
  selectedKeys: [] as string[],
  previewJson: '',
})

const genFilteredKeys = computed(() => {
  const kw = String(gen.value.keyFilter || '').trim().toLowerCase()
  if (!kw) return gen.value.keys
  return gen.value.keys.filter((k) => k.toLowerCase().includes(kw))
})

function openGen() {
  gen.value.err = ''
  const row = selectedRow.value
  if (!row) {
    gen.value.err = '请先在右侧表格中选中一条事件'
    gen.value.open = true
    return
  }
  const tags = row.tags || {}
  const keys = Object.keys(tags || {}).sort()
  gen.value.keys = keys
  // 默认选择 cluster/app/env，如果不存在则全选
  const common = ['cluster', 'app', 'env']
  const picked = common.filter((k) => keys.includes(k))
  gen.value.selectedKeys = picked.length > 0 ? picked : keys.slice(0, 20)
  gen.value.field = 'tag_regex'
  gen.value.priority = 200
  gen.value.route = String(row.route_name || '').trim()
  gen.value.name = `auto_${Date.now()}`
  gen.value.robotIDsText = ''
  buildPreview()
  gen.value.open = true
}

function genSelectAll() {
  gen.value.selectedKeys = [...gen.value.keys]
  buildPreview()
}
function genSelectNone() {
  gen.value.selectedKeys = []
  buildPreview()
}
function genSelectCommon() {
  const common = ['cluster', 'app', 'env']
  gen.value.selectedKeys = common.filter((k) => gen.value.keys.includes(k))
  buildPreview()
}

function safeParseStringArray(s: string): string[] {
  const v = String(s || '').trim()
  if (!v) return []
  return v
    .split(',')
    .map((x) => x.trim())
    .filter((x) => x.length > 0)
}

function escapeRegExp(s: string) {
  return String(s || '').replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function buildPreview() {
  gen.value.err = ''
  const row = selectedRow.value
  if (!row) {
    gen.value.previewJson = ''
    return
  }
  const tags = row.tags || {}
  const out: any = {}
  for (const k of gen.value.selectedKeys || []) {
    const vv = tags[k]
    if (vv == null) continue
    const s = String(vv)
    if (gen.value.field === 'tags') {
      out[k] = s
    } else {
      out[k] = `^${escapeRegExp(s)}$`
    }
  }
  gen.value.previewJson = JSON.stringify(out, null, 2)
}

watch(
  () => [gen.value.field, gen.value.selectedKeys, gen.value.keyFilter],
  () => {
    buildPreview()
  },
)

function saveAndGotoSettings() {
  gen.value.err = ''
  const row = selectedRow.value
  if (!row) {
    gen.value.err = '未选中事件'
    return
  }
  const robotIDs = safeParseStringArray(gen.value.robotIDsText || '')
  if (robotIDs.length === 0) {
    gen.value.err = 'robot_ids 不能为空（至少填一个 Robots.id）'
    return
  }
  const tags = row.tags || {}
  const kv: any = {}
  for (const k of gen.value.selectedKeys || []) {
    const vv = tags[k]
    if (vv == null) continue
    const s = String(vv)
    kv[k] = gen.value.field === 'tags' ? s : `^${escapeRegExp(s)}$`
  }
  if (Object.keys(kv).length === 0) {
    gen.value.err = '未生成任何 tag 条件（请至少选择一个 key 且该 key 在事件 tags 中存在）'
    return
  }

  const draft = {
    name: String(gen.value.name || '').trim() || `auto_${Date.now()}`,
    priority: Number(gen.value.priority || 0),
    enabled: true,
    robot_id: '',
    robot_ids: robotIDs,
    group_id: 0,
    group_name_regex: '',
    rule_id: 0,
    rule_name_regex: '',
    route_name: String(gen.value.route || '').trim(),
    tags: gen.value.field === 'tags' ? kv : {},
    tag_regex: gen.value.field === 'tag_regex' ? kv : {},
  }
  try {
    localStorage.setItem(BINDING_DRAFT_KEY, JSON.stringify(draft))
    ElMessage.success('已生成 binding 草稿，跳转到 Settings 导入')
  } catch (e: any) {
    gen.value.err = e?.message || String(e)
    return
  }
  gen.value.open = false
  router.push('/settings')
}

onMounted(() => {
  if (!jsonText.value) loadSampleFromStorage()
})
</script>
