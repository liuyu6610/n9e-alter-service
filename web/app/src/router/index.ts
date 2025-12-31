import { createRouter, createWebHistory } from 'vue-router'

import Dashboard from '../views/Dashboard.vue'
import Alerts from '../views/Alerts.vue'
import Routes from '../views/Routes.vue'
import Preview from '../views/Preview.vue'
import RuleEditor from '../views/RuleEditor.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'Dashboard', component: Dashboard },
    { path: '/alerts', name: 'Alerts', component: Alerts },
    { path: '/routes', name: 'Routes', component: Routes },
    { path: '/preview', name: 'Preview', component: Preview },
    { path: '/rule-editor', name: 'RuleEditor', component: RuleEditor },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

export default router
