<template>
  <el-row :gutter="12">
    <el-col :span="6">
      <el-card shadow="never">
        <div style="font-size: 12px; color: #909399">Active</div>
        <div style="font-size: 22px; font-weight: 700">{{ summary.active }}</div>
      </el-card>
    </el-col>
    <el-col :span="6">
      <el-card shadow="never">
        <div style="font-size: 12px; color: #909399">Recovered</div>
        <div style="font-size: 22px; font-weight: 700">{{ summary.recovered }}</div>
      </el-card>
    </el-col>
    <el-col :span="6">
      <el-card shadow="never">
        <div style="font-size: 12px; color: #909399">Last Pull</div>
        <div class="mono" style="font-size: 14px; font-weight: 600">{{ fmtTs(engine.last_end_unix) }}</div>
      </el-card>
    </el-col>
    <el-col :span="6">
      <el-card shadow="never">
        <div style="font-size: 12px; color: #909399">Last Error</div>
        <div style="font-size: 12px; color: #f56c6c; white-space: nowrap; overflow: hidden; text-overflow: ellipsis" :title="engine.last_error">
          {{ engine.last_error || '-' }}
        </div>
      </el-card>
    </el-col>
  </el-row>

  <el-card shadow="never" style="margin-top: 12px">
    <el-row :gutter="12" align="middle">
      <el-col :span="12">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="last_fetched">{{ engine.last_fetched }}</el-descriptions-item>
          <el-descriptions-item label="last_total">{{ engine.last_total }}</el-descriptions-item>
          <el-descriptions-item label="last_inputs">{{ engine.last_inputs }}</el-descriptions-item>
          <el-descriptions-item label="purged_recovered">{{ engine.purged_recovered }}</el-descriptions-item>
        </el-descriptions>
      </el-col>
      <el-col :span="12" style="text-align: right">
        <el-button type="primary" :loading="pulling" @click="run">手动拉取</el-button>
        <el-button :loading="loading" @click="refresh">刷新</el-button>
      </el-col>
    </el-row>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'

import { getStatus, runPull } from '../api/client'
import type { EngineStatus, StateSummary } from '../api/types'
import { fmtTs } from '../utils/time'

const loading = ref(false)
const pulling = ref(false)

const engine = reactive<EngineStatus>({
  last_start_unix: 0,
  last_end_unix: 0,
  last_error: '',
  last_fetched: 0,
  last_total: 0,
  last_inputs: 0,
  active_total: 0,
  recovered_total: 0,
  new_actives: 0,
  new_recovereds: 0,
  purged_recovered: 0,
})

const summary = reactive<StateSummary>({ active: 0, recovered: 0, total: 0 })

async function refresh() {
  loading.value = true
  try {
    const data = await getStatus()
    Object.assign(engine, data.engine)
    Object.assign(summary, data.state)
  } catch (e: any) {
    ElMessage.error(`status: ${e.message || e}`)
  } finally {
    loading.value = false
  }
}

async function run() {
  pulling.value = true
  try {
    await runPull()
    ElMessage.success('pull ok')
  } catch (e: any) {
    ElMessage.error(`pull: ${e.message || e}`)
  } finally {
    pulling.value = false
    await refresh()
  }
}

onMounted(async () => {
  await refresh()
  setInterval(refresh, 10000)
})
</script>

<style scoped>
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
}
</style>
