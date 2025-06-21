import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    component: () => import('../components/layout/MainLayout.vue'),
    children: [
      {
        path: '',
        name: 'dashboard',
        component: () => import('../views/Dashboard.vue'),
        meta: {
          title: '仪表盘',
          icon: 'dashboard'
        }
      },
      {
        path: 'emails',
        name: 'emails',
        component: () => import('../views/EmailAccounts.vue'),
        meta: {
          title: '邮箱管理',
          icon: 'mail'
        }
      },
      {
        path: 'downloads',
        name: 'downloads',
        component: () => import('../views/Downloads.vue'),
        meta: {
          title: '下载任务',
          icon: 'download'
        }
      },
      {
        path: 'statistics',
        name: 'statistics',
        component: () => import('../views/Statistics.vue'),
        meta: {
          title: '统计分析',
          icon: 'chart'
        }
      },
      {
        path: 'settings',
        name: 'settings',
        component: () => import('../views/Settings.vue'),
        meta: {
          title: '应用设置',
          icon: 'settings'
        }
      },
      {
        path: 'logs',
        name: 'logs',
        component: () => import('../views/Logs.vue'),
        meta: {
          title: '运行日志',
          icon: 'document'
        }
      }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

export default router 