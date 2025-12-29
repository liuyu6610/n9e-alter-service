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
          <div>预览结果</div>
        </template>

        <el-alert v-if="error" :title="error" type="error" show-icon style="margin-bottom: 10px" />

        <el-table :data="items" size="small" border style="width: 100%" height="520">
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
          <el-collapse-item v-for="(it, idx) in items" :key="idx" :name="String(idx)">
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
</template>

<script setup lang="ts">
import { ref } from 'vue'

import { previewRoutes } from '../api/client'

const jsonText = ref('')
const loading = ref(false)
const error = ref('')
const items = ref([] as any[])
const activeNames = ref([] as string[])

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
    activeNames.value = items.value.slice(0, 3).map((_: any, i: number) => String(i))
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    loading.value = false
  }
}
</script>
