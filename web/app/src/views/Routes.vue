<template>
  <el-card shadow="never">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px">
      <div style="font-weight: 600">路由列表</div>
      <el-button :loading="loading" @click="refresh">刷新</el-button>
    </div>

    <el-table :data="items" stripe v-loading="loading" style="width: 100%" row-key="name">
      <el-table-column prop="name" label="name" width="160" />
      <el-table-column prop="enabled" label="enabled" width="100">
        <template #default="scope">
          <el-tag :type="scope.row.enabled ? 'success' : 'info'">{{ scope.row.enabled }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="match" show-overflow-tooltip>
        <template #default="scope">
          <span class="mono">group={{ scope.row.match.group_name_regex }}</span>
          <span style="margin-left: 8px" class="mono">rule={{ scope.row.match.rule_name_regex }}</span>
        </template>
      </el-table-column>
      <el-table-column label="severity_in" width="160">
        <template #default="scope">
          <span class="mono">{{ (scope.row.match.severity_in || []).join(',') }}</span>
        </template>
      </el-table-column>
      <el-table-column label="dedup" width="260">
        <template #default="scope">
          <span class="mono">mode={{ scope.row.dedup.mode }}</span>
          <span style="margin-left: 8px" class="mono">normalize_pod={{ scope.row.dedup.normalize_pod_name }}</span>
        </template>
      </el-table-column>
      <el-table-column label="notify" width="220">
        <template #default="scope">
          <el-tag :type="scope.row.notify.enabled ? 'success' : 'info'">enabled={{ scope.row.notify.enabled }}</el-tag>
          <span style="margin-left: 8px" class="mono">observe={{ scope.row.notify.observe_seconds }}s</span>
        </template>
      </el-table-column>
      <el-table-column label="daily_report" width="300">
        <template #default="scope">
          <el-tag :type="scope.row.daily_report.enabled ? 'success' : 'info'">enabled={{ scope.row.daily_report.enabled }}</el-tag>
          <span style="margin-left: 8px" class="mono">cron={{ scope.row.daily_report.cron }}</span>
        </template>
      </el-table-column>
      <el-table-column type="expand">
        <template #default="scope">
          <el-descriptions :column="2" border>
            <el-descriptions-item label="title_prefix">{{ scope.row.daily_report.title_prefix }}</el-descriptions-item>
            <el-descriptions-item label="clear_mode">{{ scope.row.daily_report.clear_mode }}</el-descriptions-item>
            <el-descriptions-item label="max_lines">{{ scope.row.daily_report.max_lines }}</el-descriptions-item>
            <el-descriptions-item label="max_chars">{{ scope.row.daily_report.max_chars }}</el-descriptions-item>
            <el-descriptions-item label="repeat_interval_seconds">{{ scope.row.notify.repeat_interval_seconds }}</el-descriptions-item>
            <el-descriptions-item label="send_recovered">{{ scope.row.notify.send_recovered }}</el-descriptions-item>
          </el-descriptions>

          <div style="margin-top: 12px; font-weight: 600">dedup keys</div>
          <pre class="mono" style="white-space: pre-wrap; margin: 0">{{ JSON.stringify(scope.row.dedup, null, 2) }}</pre>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'

import { getRoutes } from '../api/client'
import type { RouteItem } from '../api/types'

const loading = ref(false)
const items = ref<RouteItem[]>([])

async function refresh() {
  loading.value = true
  try {
    const data = await getRoutes()
    items.value = data.items || []
  } catch (e: any) {
    ElMessage.error(`routes: ${e.message || e}`)
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await refresh()
})
</script>

<style scoped>
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
}
</style>
