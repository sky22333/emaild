<template>
  <div class="downloads">
    <!-- 页面头部 -->
    <div class="page-header">
      <div class="page-title">
        <h2>📥 下载管理</h2>
        <p>管理所有PDF文件下载任务</p>
      </div>
      
      <div class="page-actions">
        <n-button-group>
          <n-button @click="refreshTasks" :loading="isLoading">
            刷新
          </n-button>
          <n-button @click="startAll" :disabled="!hasWaitingTasks">
            全部开始
          </n-button>
          <n-button @click="pauseAll" :disabled="!hasRunningTasks">
            全部暂停
          </n-button>
          <n-button @click="clearCompleted" :disabled="!hasCompletedTasks">
            清除已完成
          </n-button>
        </n-button-group>
      </div>
    </div>

    <!-- 统计信息 -->
    <n-card class="stats-card" size="small">
      <n-space>
        <n-statistic label="总计" :value="downloadStats.total" />
        <n-statistic label="下载中" :value="downloadStats.running" />
        <n-statistic label="等待中" :value="downloadStats.waiting" />
        <n-statistic label="已完成" :value="downloadStats.completed" />
        <n-statistic label="失败" :value="downloadStats.failed" />
      </n-space>
    </n-card>

    <!-- 筛选和搜索 -->
    <n-card class="filter-card" size="small">
      <n-space>
        <n-select
          v-model:value="statusFilter"
          :options="statusOptions"
          placeholder="按状态筛选"
          style="width: 120px;"
          clearable
        />
        <n-select
          v-model:value="emailFilter"
          :options="emailOptions"
          placeholder="按邮箱筛选"
          style="width: 200px;"
          clearable
        />
        <n-input
          v-model:value="searchKeyword"
          placeholder="搜索文件名..."
          style="width: 200px;"
          clearable
        />
      </n-space>
    </n-card>

    <!-- 错误提示 -->
    <n-alert v-if="error" type="error" :title="error.message" closable @close="clearError" />

    <!-- 任务列表 -->
    <div class="task-list">
      <n-card
        v-for="task in filteredTasks"
        :key="task.id"
        class="task-card"
        :class="getTaskClass(task.status)"
      >
        <div class="task-header">
          <div class="task-info">
            <div class="task-icon">
              <span class="task-icon-emoji" :class="getTaskIconClass(task.status)">
                {{ getTaskIcon(task.status) }}
              </span>
            </div>
            <div class="task-details">
              <div class="task-filename">{{ task.file_name }}</div>
              <div class="task-meta">
                <span class="task-email">来自: {{ task.sender }}</span>
                <span class="task-size">{{ formatFileSize(task.file_size) }}</span>
                <span class="task-time">{{ formatTime(task.created_at) }}</span>
              </div>
            </div>
          </div>
          
          <div class="task-actions">
            <n-tag :type="getStatusTagType(task.status)" size="small">
              {{ getStatusText(task.status) }}
            </n-tag>
            
            <n-dropdown
              :options="getTaskActions(task)"
              @select="handleTaskAction($event, task)"
              placement="bottom-end"
            >
              <n-button quaternary circle size="small">
                <template #icon>
                  <span style="font-size: 16px;">⋮</span>
                </template>
              </n-button>
            </n-dropdown>
          </div>
        </div>

        <!-- 进度条 -->
        <div v-if="task.status === 'downloading'" class="task-progress">
          <n-progress
            type="line"
            :percentage="task.progress"
            :show-indicator="true"
            :indicator-placement="'inside'"
          />
          <div class="progress-info">
            <span class="progress-speed">{{ task.speed }}</span>
            <span class="progress-size">{{ formatFileSize(task.downloaded_size) }} / {{ formatFileSize(task.file_size) }}</span>
          </div>
        </div>

        <!-- 错误信息 -->
        <div v-if="(task.status === 'failed' || task.status === 'cancelled') && task.error" class="task-error">
          <n-alert type="error" size="small" :show-icon="false">
            {{ task.error }}
          </n-alert>
        </div>

        <!-- 任务详情展开 -->
        <Transition name="expand">
          <div v-if="task.expanded" class="task-details-expanded">
            <n-divider />
            <div class="task-detail-grid">
              <div class="detail-item">
                <span class="detail-label">邮件主题:</span>
                <span class="detail-value">{{ task.subject }}</span>
              </div>
              <div class="detail-item">
                <span class="detail-label">下载源:</span>
                <span class="detail-value">{{ task.source }}</span>
              </div>
              <div class="detail-item">
                <span class="detail-label">保存路径:</span>
                <span class="detail-value">{{ task.local_path }}</span>
              </div>
              <div class="detail-item">
                <span class="detail-label">文件类型:</span>
                <span class="detail-value">{{ task.type === 'attachment' ? '邮件附件' : '邮件链接' }}</span>
              </div>
              <div class="detail-item">
                <span class="detail-label">创建时间:</span>
                <span class="detail-value">{{ formatFullTime(task.created_at) }}</span>
              </div>
              <div v-if="task.status === 'completed'" class="detail-item">
                <span class="detail-label">完成时间:</span>
                <span class="detail-value">{{ formatFullTime(task.updated_at) }}</span>
              </div>
            </div>
          </div>
        </Transition>

        <div class="expand-toggle" @click="toggleTaskDetails(task)">
          <span style="font-size: 14px; transition: transform 0.3s;" :class="{ rotated: task.expanded }">
            🔽
          </span>
        </div>
      </n-card>

      <!-- 空状态 -->
      <div v-if="filteredTasks.length === 0" class="empty-state">
        <span class="empty-icon" style="font-size: 64px;">⬇️</span>
        <h3>{{ searchKeyword ? '没有找到匹配的任务' : '暂无下载任务' }}</h3>
        <p>{{ searchKeyword ? '尝试调整筛选条件或搜索关键词' : '当检测到新的PDF文件时，下载任务会自动出现在这里' }}</p>
      </div>
    </div>

    <!-- 分页 -->
    <div v-if="filteredTasks.length > 0" class="pagination">
      <n-pagination
        v-model:page="currentPage"
        :page-count="totalPages"
        :page-size="pageSize"
        show-size-picker
        :page-sizes="[10, 20, 50, 100]"
        @update:page-size="handlePageSizeChange"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useAppStore } from '../stores/app'
import { useErrorHandler } from '../composables/useErrorHandler'
import {
  NButton,
  NButtonGroup,
  NSpace,
  NSelect,
  NInput,
  NCard,
  NTag,
  NDropdown,
  NProgress,
  NAlert,
  NDivider,
  NPagination,
  NStatistic
} from 'naive-ui'
import type { DownloadTask } from '../wails'

const appStore = useAppStore()
const { withErrorHandling, isLoading, error, clearError } = useErrorHandler()

// 响应式数据
const statusFilter = ref('')
const emailFilter = ref('')
const searchKeyword = ref('')
const currentPage = ref(1)
const pageSize = ref(20)

// 计算属性 - 修正字段名
const downloadStats = computed(() => {
  const tasks = appStore.downloadTasks || []
  return {
    total: tasks.length,
    running: tasks.filter(t => t.status === 'downloading').length,
    waiting: tasks.filter(t => t.status === 'pending').length,
    completed: tasks.filter(t => t.status === 'completed').length,
    failed: tasks.filter(t => t.status === 'failed' || t.status === 'cancelled').length
  }
})

const filteredTasks = computed(() => {
  let tasks = appStore.downloadTasks || []
  
  // 状态筛选
  if (statusFilter.value) {
    tasks = tasks.filter(task => task.status === statusFilter.value)
  }
  
  // 邮箱筛选  
  if (emailFilter.value) {
    tasks = tasks.filter(task => task.sender.includes(emailFilter.value))
  }
  
  // 关键词搜索
  if (searchKeyword.value) {
    const keyword = searchKeyword.value.toLowerCase()
    tasks = tasks.filter(task => 
      task.file_name.toLowerCase().includes(keyword) ||
      task.sender.toLowerCase().includes(keyword) ||
      task.subject.toLowerCase().includes(keyword)
    )
  }
  
  // 分页
  const start = (currentPage.value - 1) * pageSize.value
  const end = start + pageSize.value
  return tasks.slice(start, end)
})

const totalPages = computed(() => {
  const total = appStore.downloadTasks?.length || 0
  return Math.ceil(total / pageSize.value)
})

const hasWaitingTasks = computed(() => downloadStats.value.waiting > 0)
const hasRunningTasks = computed(() => downloadStats.value.running > 0)
const hasCompletedTasks = computed(() => downloadStats.value.completed > 0)

const statusOptions = [
  { label: '全部', value: '' },
  { label: '等待中', value: 'pending' },
  { label: '下载中', value: 'downloading' },
  { label: '已完成', value: 'completed' },
  { label: '失败', value: 'failed' },
  { label: '暂停', value: 'paused' },
  { label: '已取消', value: 'cancelled' }
]

const emailOptions = computed(() => {
  const senders = [...new Set((appStore.downloadTasks || []).map(task => task.sender))]
  return [
    { label: '全部邮箱', value: '' },
    ...senders.map(sender => ({ label: sender, value: sender }))
  ]
})

// 方法
const refreshTasks = async () => {
  await withErrorHandling(async () => {
    await appStore.loadDownloadTasks()
  }, '刷新任务列表')
}

const startAll = async () => {
  await withErrorHandling(async () => {
    const waitingTasks = appStore.downloadTasks.filter(t => t.status === 'pending' || t.status === 'paused')
    for (const task of waitingTasks) {
      await appStore.resumeTask(task.id)
    }
    await refreshTasks()
  }, '开始所有任务')
}

const pauseAll = async () => {
  await withErrorHandling(async () => {
    const runningTasks = appStore.downloadTasks.filter(t => t.status === 'downloading')
    for (const task of runningTasks) {
      await appStore.pauseTask(task.id)
    }
    await refreshTasks()
  }, '暂停所有任务')
}

const clearCompleted = async () => {
  await withErrorHandling(async () => {
    // 这里需要后端支持批量删除已完成任务的API
    console.log('清除已完成任务功能待实现')
  }, '清除已完成任务')
}

const getTaskClass = (status: string) => {
  return `task-${status}`
}

const getTaskIcon = (status: string) => {
  const iconMap: Record<string, string> = {
    pending: '⏳',
    downloading: '⬇️',
    completed: '✅',
    failed: '❌',
    paused: '⏸️',
    cancelled: '🚫'
  }
  return iconMap[status] || '📄'
}

const getTaskIconClass = (status: string) => {
  return `icon-${status}`
}

const getStatusText = (status: string) => {
  const textMap: Record<string, string> = {
    pending: '等待中',
    downloading: '下载中',
    completed: '已完成',
    failed: '失败',
    paused: '已暂停',
    cancelled: '已取消'
  }
  return textMap[status] || status
}

const getStatusTagType = (status: string) => {
  const typeMap: Record<string, string> = {
    pending: 'default',
    downloading: 'info',
    completed: 'success',
    failed: 'error',
    paused: 'warning',
    cancelled: 'default'
  }
  return typeMap[status] || 'default'
}

const getTaskActions = (task: DownloadTask) => {
  const actions = []
  
  if (task.status === 'pending' || task.status === 'paused') {
    actions.push({ label: '开始下载', key: 'start' })
  }
  
  if (task.status === 'downloading') {
    actions.push({ label: '暂停下载', key: 'pause' })
  }
  
  if (task.status === 'failed') {
    actions.push({ label: '重试下载', key: 'retry' })
  }
  
  if (task.status === 'completed') {
    actions.push({ label: '打开文件', key: 'open' })
    actions.push({ label: '打开文件夹', key: 'openFolder' })
  }
  
  if (task.status !== 'downloading') {
    actions.push({ label: '取消任务', key: 'cancel' })
  }
  
  return actions
}

const handleTaskAction = async (key: string, task: DownloadTask) => {
  await withErrorHandling(async () => {
    switch (key) {
      case 'start':
      case 'retry':
        await appStore.resumeTask(task.id)
        break
      case 'pause':
        await appStore.pauseTask(task.id)
        break
      case 'cancel':
        await appStore.cancelTask(task.id)
        break
      case 'open':
        await appStore.openFile(task.local_path)
        break
      case 'openFolder':
        await appStore.openDownloadFolder()
        break
    }
    await refreshTasks()
  }, `执行操作: ${key}`)
}

const toggleTaskDetails = (task: DownloadTask & { expanded?: boolean }) => {
  task.expanded = !task.expanded
}

const handlePageSizeChange = (newPageSize: number) => {
  pageSize.value = newPageSize
  currentPage.value = 1
}

// 格式化函数
const formatFileSize = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const formatTime = (time: string): string => {
  const now = new Date()
  const targetTime = new Date(time)
  const diff = now.getTime() - targetTime.getTime()
  
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)}分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}小时前`
  return `${Math.floor(diff / 86400000)}天前`
}

const formatFullTime = (time: string): string => {
  return new Date(time).toLocaleString()
}

// 生命周期
onMounted(() => {
  refreshTasks()
})
</script>

<style scoped>
.downloads {
  padding: 24px;
  max-width: 1200px;
  margin: 0 auto;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 24px;
}

.page-title h2 {
  margin: 0 0 8px 0;
  font-size: 24px;
  font-weight: 600;
}

.page-title p {
  margin: 0;
  color: #666;
  font-size: 14px;
}

.stats-card,
.filter-card {
  margin-bottom: 16px;
}

.task-list {
  margin-bottom: 24px;
}

.task-card {
  margin-bottom: 12px;
  position: relative;
}

.task-card:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.task-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.task-info {
  display: flex;
  align-items: flex-start;
  flex: 1;
}

.task-icon {
  margin-right: 12px;
  padding-top: 2px;
}

.task-icon-emoji {
  font-size: 20px;
}

.task-details {
  flex: 1;
}

.task-filename {
  font-weight: 500;
  font-size: 16px;
  margin-bottom: 4px;
  word-break: break-all;
}

.task-meta {
  display: flex;
  gap: 16px;
  font-size: 12px;
  color: #666;
}

.task-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.task-progress {
  margin-top: 12px;
}

.progress-info {
  display: flex;
  justify-content: space-between;
  margin-top: 4px;
  font-size: 12px;
  color: #666;
}

.task-error {
  margin-top: 8px;
}

.task-details-expanded {
  margin-top: 12px;
}

.task-detail-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 8px;
  font-size: 14px;
}

.detail-item {
  display: flex;
  gap: 8px;
}

.detail-label {
  font-weight: 500;
  color: #666;
  min-width: 80px;
}

.detail-value {
  word-break: break-all;
  flex: 1;
}

.expand-toggle {
  position: absolute;
  right: 8px;
  bottom: 8px;
  cursor: pointer;
  padding: 4px;
  border-radius: 4px;
  transition: background-color 0.2s;
}

.expand-toggle:hover {
  background-color: rgba(0, 0, 0, 0.05);
}

.rotated {
  transform: rotate(180deg);
}

.empty-state {
  text-align: center;
  padding: 64px 24px;
  color: #666;
}

.empty-state h3 {
  margin: 16px 0 8px 0;
  font-size: 18px;
  font-weight: 500;
}

.empty-state p {
  margin: 0;
  font-size: 14px;
}

.pagination {
  display: flex;
  justify-content: center;
}

.expand-enter-active,
.expand-leave-active {
  transition: all 0.3s ease;
  overflow: hidden;
}

.expand-enter-from,
.expand-leave-to {
  max-height: 0;
  opacity: 0;
}

.expand-enter-to,
.expand-leave-from {
  max-height: 500px;
  opacity: 1;
}

/* 状态相关样式 */
.task-pending {
  border-left: 4px solid #ccc;
}

.task-downloading {
  border-left: 4px solid #2080f0;
}

.task-completed {
  border-left: 4px solid #18a058;
}

.task-failed {
  border-left: 4px solid #d03050;
}

.task-paused {
  border-left: 4px solid #f0a020;
}

.task-cancelled {
  border-left: 4px solid #666;
}
</style> 
