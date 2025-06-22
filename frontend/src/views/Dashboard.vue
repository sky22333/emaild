<template>
  <div class="dashboard">
    <n-spin :show="isLoading">
      <n-card title="📊 仪表板">
        <n-space vertical>
          <!-- 错误提示 -->
          <n-alert v-if="error" type="error" :title="error.message" closable @close="clearError" />
          
          <!-- 欢迎信息 -->
          <n-alert type="success" title="欢迎使用">
            邮件附件下载器已成功启动！系统正在运行中...
          </n-alert>
          
          <!-- 统计卡片 -->
          <n-grid :cols="4" :x-gap="12">
            <n-gi>
              <n-card title="📧 邮箱账户" size="small">
                <template #header-extra>
                  <n-button size="small" @click="checkAllEmails" :loading="isLoading">
                    检查邮件
                  </n-button>
                </template>
                <n-statistic 
                  label="活跃账户" 
                  :value="stats.activeAccounts" 
                  :precision="0"
                />
                <n-statistic 
                  label="总计" 
                  :value="stats.totalAccounts" 
                  :precision="0"
                  style="margin-top: 8px;"
                />
              </n-card>
            </n-gi>
            
            <n-gi>
              <n-card title="⬇️ 下载任务" size="small">
                <template #header-extra>
                  <n-button size="small" @click="viewDownloads">
                    查看全部
                  </n-button>
                </template>
                <n-statistic 
                  label="进行中" 
                  :value="stats.runningTasks" 
                  :precision="0"
                />
                <n-statistic 
                  label="总计" 
                  :value="stats.totalTasks" 
                  :precision="0"
                  style="margin-top: 8px;"
                />
              </n-card>
            </n-gi>
            
            <n-gi>
              <n-card title="✅ 完成情况" size="small">
                <n-statistic 
                  label="已完成" 
                  :value="stats.completedTasks" 
                  :precision="0"
                />
                <n-progress 
                  :percentage="completionRate" 
                  :color="completionRate > 80 ? '#18a058' : '#f0a020'"
                  style="margin-top: 8px;"
                />
              </n-card>
            </n-gi>
            
            <n-gi>
              <n-card title="❌ 失败任务" size="small">
                <template #header-extra>
                  <n-button size="small" @click="openDownloadFolder">
                    打开文件夹
                  </n-button>
                </template>
                <n-statistic 
                  label="失败" 
                  :value="stats.failedTasks" 
                  :precision="0"
                />
              </n-card>
            </n-gi>
          </n-grid>

          <!-- 服务状态 -->
          <n-card title="🔧 服务状态" size="small">
            <n-space>
              <n-tag 
                :type="appStore.serviceStatus.email ? 'success' : 'error'"
                round
              >
                邮件监控: {{ appStore.serviceStatus.email ? '运行中' : '已停止' }}
              </n-tag>
              <n-tag 
                :type="appStore.serviceStatus.download ? 'success' : 'error'"
                round
              >
                下载服务: {{ appStore.serviceStatus.download ? '运行中' : '已停止' }}
              </n-tag>
            </n-space>
          </n-card>

          <!-- 最近任务 -->
          <n-card title="📋 最近任务" size="small">
            <n-list v-if="recentTasks.length > 0">
              <n-list-item v-for="task in recentTasks" :key="task.id">
                <n-thing 
                  :title="task.file_name" 
                  :description="`来自: ${task.sender} | 主题: ${task.subject}`"
                >
                  <template #header-extra>
                    <n-space>
                      <n-tag :type="getStatusColor(task.status)">
                        {{ getStatusText(task.status) }}
                      </n-tag>
                      <span class="text-gray-500">
                        {{ formatFileSize(task.file_size) }}
                      </span>
                    </n-space>
                  </template>
                  <template #footer>
                    <n-progress 
                      v-if="task.status === 'downloading'"
                      :percentage="task.progress" 
                      :show-indicator="false"
                      style="margin-top: 8px;"
                    />
                    <n-time 
                      v-else
                      :time="new Date(task.created_at)" 
                      format="MM-dd HH:mm"
                      style="color: #999; font-size: 12px;"
                    />
                  </template>
                </n-thing>
              </n-list-item>
            </n-list>
            <n-empty v-else description="暂无下载任务" />
          </n-card>
        </n-space>
      </n-card>
    </n-spin>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useAppStore } from '../stores/app'
import { useErrorHandler } from '../composables/useErrorHandler'
import { 
  NCard, 
  NStatistic, 
  NGrid, 
  NGi, 
  NButton, 
  NSpace, 
  NSpin,
  NAlert,
  NProgress,
  NList,
  NListItem,
  NThing,
  NTag,
  NTime,
  NEmpty
} from 'naive-ui'
import type { DownloadTask } from '../composables/useApi'

const router = useRouter()
const appStore = useAppStore()
const { withErrorHandling, isLoading, error, clearError } = useErrorHandler()

// 响应式数据
const stats = ref({
  totalAccounts: 0,
  activeAccounts: 0,
  totalTasks: 0,
  completedTasks: 0,
  runningTasks: 0,
  failedTasks: 0
})

// 计算属性
const completionRate = computed(() => {
  if (stats.value.totalTasks === 0) return 0
  return Math.round((stats.value.completedTasks / stats.value.totalTasks) * 100)
})

const dashboardStats = computed(() => {
  const accounts = appStore.emailAccounts || []
  const tasks = appStore.downloadTasks || []
  
  return {
    totalAccounts: accounts.length,
    activeAccounts: accounts.filter(a => a.is_active).length,
    totalTasks: tasks.length,
    runningTasks: tasks.filter(t => t.status === 'downloading').length,
    completedTasks: tasks.filter(t => t.status === 'completed').length,
    failedTasks: tasks.filter(t => t.status === 'failed').length
  }
})

const recentTasks = computed(() => {
  const tasks = appStore.downloadTasks || []
  return tasks
    .slice()
    .sort((a, b) => new Date(b.created_at || 0).getTime() - new Date(a.created_at || 0).getTime())
    .slice(0, 5)
})

const chartData = computed(() => {
  const tasks = appStore.downloadTasks || []
  const last7Days = []
  const now = new Date()
  
  for (let i = 6; i >= 0; i--) {
    const date = new Date(now)
    date.setDate(date.getDate() - i)
    const dateStr = date.toISOString().split('T')[0]
    
    const dayTasks = tasks.filter(task => {
      const taskDate = new Date(task.created_at || 0).toISOString().split('T')[0]
      return taskDate === dateStr
    })
    
    last7Days.push({
      date: dateStr,
      completed: dayTasks.filter(t => t.status === 'completed').length,
      failed: dayTasks.filter(t => t.status === 'failed').length
    })
  }
  
  return last7Days
})

// 方法
const loadDashboardData = async () => {
  await withErrorHandling(async () => {
    // 并行加载数据
    await Promise.all([
      appStore.loadEmailAccounts(),
      appStore.loadDownloadTasks(1, 10),
      appStore.checkServiceStatus()
    ])
    
    // 更新统计数据
    stats.value = {
      totalAccounts: appStore.emailAccounts.length,
      activeAccounts: appStore.activeEmailAccounts.length,
      totalTasks: appStore.downloadTasks.length,
      completedTasks: appStore.completedTasks.length,
      runningTasks: appStore.runningTasks.length,
      failedTasks: appStore.failedTasks.length
    }
    
    // 获取最近的任务
    recentTasks.value = appStore.downloadTasks.slice(0, 5)
  }, '加载仪表板数据')
}

const checkAllEmails = async () => {
  await withErrorHandling(async () => {
    await appStore.checkAllEmails()
    await loadDashboardData()
  }, '检查邮件')
}

const openDownloadFolder = async () => {
  await withErrorHandling(async () => {
    await appStore.openDownloadFolder()
  }, '打开下载文件夹')
}

const viewDownloads = () => {
  router.push({ name: 'downloads' })
}

const formatFileSize = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const getStatusColor = (status: string) => {
  const colors = {
    'completed': 'success',
    'downloading': 'info',
    'failed': 'error',
    'pending': 'warning',
    'paused': 'default',
    'cancelled': 'default'
  }
  return colors[status as keyof typeof colors] || 'default'
}

const getStatusText = (status: string) => {
  const texts = {
    'completed': '已完成',
    'downloading': '下载中',
    'failed': '失败',
    'pending': '等待中',
    'paused': '已暂停',
    'cancelled': '已取消'
  }
  return texts[status as keyof typeof texts] || status
}

// 生命周期
onMounted(() => {
  loadDashboardData()
})
</script>

<style scoped>
.dashboard {
  padding: 24px;
  max-width: 1200px;
  margin: 0 auto;
}

.text-gray-500 {
  color: #6b7280;
  font-size: 12px;
}
</style> 
