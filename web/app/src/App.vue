<template>
  <el-container style="height: 100vh">
    <el-aside width="220px" style="background: #001529">
      <div style="height: 56px; display: flex; align-items: center; padding: 0 16px; color: #fff; font-weight: 700">
        n9e-alter-service
      </div>
      <el-menu
        router
        :default-active="active"
        background-color="#001529"
        text-color="#cfd3dc"
        active-text-color="#fff"
      >
        <el-menu-item index="/">
          <el-icon><DataBoard /></el-icon>
          <span>仪表盘</span>
        </el-menu-item>
        <el-menu-item index="/alerts">
          <el-icon><Bell /></el-icon>
          <span>告警</span>
        </el-menu-item>
        <el-menu-item index="/routes">
          <el-icon><Operation /></el-icon>
          <span>路由</span>
        </el-menu-item>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header style="background: #fff; border-bottom: 1px solid #ebeef5">
        <div style="display: flex; align-items: center; justify-content: space-between; height: 100%">
          <div style="font-weight: 600">{{ title }}</div>
          <div style="display: flex; gap: 10px; align-items: center">
            <el-tag :type="health.ok ? 'success' : 'danger'">health: {{ health.text }}</el-tag>
            <el-tag :type="ready.ok ? 'success' : 'danger'">ready: {{ ready.text }}</el-tag>
          </div>
        </div>
      </el-header>
      <el-main style="padding: 16px">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive } from 'vue'
import { useRoute } from 'vue-router'
import { Bell, DataBoard, Operation } from '@element-plus/icons-vue'

import { getHealth, getReady } from './api/client'

const route = useRoute()

const active = computed(() => route.path)

const title = computed(() => {
  if (route.path === '/alerts') return '告警'
  if (route.path === '/routes') return '路由'
  return '仪表盘'
})

const health = reactive({ ok: false, text: 'unknown' })
const ready = reactive({ ok: false, text: 'unknown' })

async function refresh() {
  try {
    const h = await getHealth()
    health.ok = true
    health.text = h.status || 'ok'
  } catch {
    health.ok = false
    health.text = 'error'
  }

  try {
    const r = await getReady()
    ready.ok = true
    ready.text = r.status || 'ok'
  } catch {
    ready.ok = false
    ready.text = 'error'
  }
}

onMounted(async () => {
  await refresh()
  setInterval(refresh, 10000)
})
</script>
