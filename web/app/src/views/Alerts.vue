<template>
  <el-card shadow="never">
    <el-form :inline="true" style="margin-bottom: 12px">
      <el-form-item label="route">
        <el-select v-model="q.route" clearable filterable placeholder="route" style="width: 220px" :loading="routesLoading">
          <el-option v-for="it in routeOptions" :key="it.value" :label="it.label" :value="it.value" />
        </el-select>
      </el-form-item>
      <el-form-item label="status">
        <el-radio-group v-model="q.status">
          <el-radio-button label="active">active</el-radio-button>
          <el-radio-button label="recovered">recovered</el-radio-button>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="pageSize">
        <el-input-number v-model="q.pageSize" :min="10" :max="100000" :step="10" />
      </el-form-item>
      <el-form-item>
        <el-button :loading="loading" @click="refresh">刷新</el-button>
      </el-form-item>
    </el-form>

    <el-table :data="items" stripe v-loading="loading" style="width: 100%" @row-dblclick="openDetail" row-key="service_hash">
      <el-table-column prop="severity" label="P" width="70">
        <template #default="scope">
          <el-tag :type="severityType(scope.row.severity)">P{{ scope.row.severity }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="group_name" label="业务组" width="180" show-overflow-tooltip />
      <el-table-column prop="rule_name" label="规则" show-overflow-tooltip />
      <el-table-column prop="entity" label="对象" show-overflow-tooltip />
      <el-table-column prop="raw_count" label="计数" width="80" />
      <el-table-column prop="miss_count" label="miss" width="80" />
      <el-table-column prop="first_seen_at" label="首次发现" width="160">
        <template #default="scope"><span class="mono">{{ fmtTs(scope.row.first_seen_at) }}</span></template>
      </el-table-column>
      <el-table-column prop="last_seen_at" label="最新发现" width="160">
        <template #default="scope"><span class="mono">{{ fmtTs(scope.row.last_seen_at) }}</span></template>
      </el-table-column>
      <el-table-column prop="last_trigger_time" label="最新触发" width="160">
        <template #default="scope"><span class="mono">{{ fmtTs(scope.row.last_trigger_time) }}</span></template>
      </el-table-column>
      <el-table-column prop="last_notified" label="最近通知" width="160">
        <template #default="scope"><span class="mono">{{ fmtTs(scope.row.last_notified) }}</span></template>
      </el-table-column>
      <el-table-column prop="recovered_at" label="恢复时间" width="160">
        <template #default="scope"><span class="mono">{{ fmtTs(scope.row.recovered_at) }}</span></template>
      </el-table-column>
      <el-table-column prop="service_hash" label="Hash" width="220" show-overflow-tooltip>
        <template #default="scope"><span class="mono">{{ (scope.row.service_hash || '').slice(0, 16) }}</span></template>
      </el-table-column>
      <el-table-column label="操作" width="100" fixed="right">
        <template #default="scope">
          <el-button link type="primary" @click="openDetail(scope.row)">详情</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div style="display: flex; justify-content: flex-end; margin-top: 12px">
      <el-pagination
        background
        layout="total, sizes, prev, pager, next"
        :total="total"
        v-model:current-page="q.page"
        v-model:page-size="q.pageSize"
        :page-sizes="[10, 20, 50, 100, 200, 500, 1000]"
        @current-change="refresh"
        @size-change="onSizeChange"
      />
    </div>
  </el-card>

  <el-drawer v-model="drawer.open" title="事件详情" size="50%">
    <div v-if="drawer.loading">loading...</div>
    <pre v-else class="mono" style="white-space: pre-wrap">{{ drawer.item ? JSON.stringify(drawer.item, null, 2) : 'empty' }}</pre>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'

import { getAlert, getRoutes, listAlerts } from '../api/client'
import type { RecordItem, RouteItem } from '../api/types'
import { fmtTs } from '../utils/time'

const loading = ref(false)
const routesLoading = ref(false)

const routes = ref<RouteItem[]>([])
const routeOptions = computed(() => routes.value.filter(r => r.enabled).map(r => ({ label: r.name, value: r.name })))

const q = reactive({
  status: 'active',
  route: '',
  page: 1,
  pageSize: 100,
})

const items = ref<RecordItem[]>([])
const total = ref(0)

const drawer = reactive({ open: false, loading: false, item: null as RecordItem | null })

function severityType(sev: number) {
  if (sev <= 1) return 'danger'
  if (sev === 2) return 'warning'
  if (sev === 3) return 'info'
  return ''
}

async function refreshRoutes() {
  routesLoading.value = true
  try {
    const data = await getRoutes()
    routes.value = data.items || []
  } catch (e: any) {
    ElMessage.error(`routes: ${e.message || e}`)
  } finally {
    routesLoading.value = false
  }
}

async function refresh() {
  loading.value = true
  try {
    const offset = (q.page - 1) * q.pageSize
    const data = await listAlerts({ status: q.status, route: q.route || undefined, offset, limit: q.pageSize })
    items.value = data.items || []
    total.value = data.total || 0
  } catch (e: any) {
    ElMessage.error(`alerts: ${e.message || e}`)
  } finally {
    loading.value = false
  }
}

async function onSizeChange() {
  q.page = 1
  await refresh()
}

async function openDetail(row: RecordItem) {
  if (!row || !row.service_hash) return
  drawer.open = true
  drawer.loading = true
  drawer.item = null
  try {
    const data = await getAlert(row.service_hash)
    drawer.item = data.item
  } catch (e: any) {
    ElMessage.error(`detail: ${e.message || e}`)
  } finally {
    drawer.loading = false
  }
}

watch(
  () => [q.status, q.route],
  async () => {
    q.page = 1
    await refresh()
  },
)

onMounted(async () => {
  await refreshRoutes()
  await refresh()
})
</script>

<style scoped>
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
}
</style>
