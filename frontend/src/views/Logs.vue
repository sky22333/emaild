<template>
  <div class="logs">
    <!-- 页面标题 -->
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">运行日志</h1>
        <p class="page-description">查看应用运行日志、错误信息和调试输出</p>
      </div>
      <div class="header-right">
        <n-space>
          <n-select
            v-model:value="selectedLevel"
            :options="logLevelOptions"
            placeholder="日志级别"
            style="width: 120px"
            clearable
          />
          <n-date-picker
            v-model:value="dateRange"
            type="daterange"
            clearable
            style="width: 300px"
          />
          <n-input
            v-model:value="searchKeyword"
            placeholder="搜索日志内容..."
            style="width: 200px"
            clearable
          >
            <template #prefix>
              <n-icon><SearchIcon /></n-icon>
            </template>
          </n-input>
          <n-button @click="refreshLogs" :loading="loading">
            <template #icon>
              <n-icon><RefreshIcon /></n-icon>
            </template>
            刷新
          </n-button>
          <n-button @click="clearLogs">
            <template #icon>
              <n-icon><DeleteIcon /></n-icon>
            </template>
            清空日志
          </n-button>
          <n-button @click="exportLogs">
            <template #icon>
              <n-icon><DownloadIcon /></n-icon>
            </template>
            导出日志
          </n-button>
        </n-space>
      </div>
    </div>

    <!-- 日志统计 -->
    <div class="log-stats">
      <div class="stat-item">
        <n-tag type="default">全部: {{ logStats.total }}</n-tag>
      </div>
      <div class="stat-item">
        <n-tag type="info">信息: {{ logStats.info }}</n-tag>
      </div>
      <div class="stat-item">
        <n-tag type="warning">警告: {{ logStats.warn }}</n-tag>
      </div>
      <div class="stat-item">
        <n-tag type="error">错误: {{ logStats.error }}</n-tag>
      </div>
      <div class="stat-item">
        <n-tag type="success">调试: {{ logStats.debug }}</n-tag>
      </div>
    </div>

    <!-- 日志列表 -->
    <div class="log-container">
      <div class="log-controls">
        <n-space>
          <n-switch v-model:value="autoScroll">
            <template #checked>自动滚动</template>
            <template #unchecked>手动滚动</template>
          </n-switch>
          <n-switch v-model:value="wordWrap">
            <template #checked>自动换行</template>
            <template #unchecked>不换行</template>
          </n-switch>
          <n-switch v-model:value="showTimestamp">
            <template #checked>显示时间</template>
            <template #unchecked>隐藏时间</template>
          </n-switch>
          <n-switch v-model:value="compactMode">
            <template #checked>紧凑模式</template>
            <template #unchecked>常规模式</template>
          </n-switch>
        </n-space>
      </div>

      <div 
        class="log-content"
        :class="{ 
          'word-wrap': wordWrap,
          'compact-mode': compactMode
        }"
        ref="logContentRef"
      >
        <div
          v-for="(log, index) in filteredLogs"
          :key="log.id"
          class="log-entry"
          :class="getLogLevelClass(log.level)"
        >
          <div class="log-header">
            <div class="log-meta">
              <span v-if="showTimestamp" class="log-time">
                {{ formatTime(log.timestamp) }}
              </span>
              <n-tag
                :type="getLogTagType(log.level)"
                size="small"
                class="log-level"
              >
                {{ log.level.toUpperCase() }}
              </n-tag>
              <span v-if="log.source" class="log-source">{{ log.source }}</span>
            </div>
            <div class="log-actions">
              <n-button
                text
                size="small"
                @click="copyLogEntry(log)"
              >
                <template #icon>
                  <n-icon><CopyIcon /></n-icon>
                </template>
              </n-button>
              <n-button
                text
                size="small"
                @click="toggleLogDetails(index)"
              >
                <template #icon>
                  <n-icon>
                    <ChevronDownIcon :class="{ rotated: log.expanded }" />
                  </n-icon>
                </template>
              </n-button>
            </div>
          </div>
          
          <div class="log-message">
            {{ log.message }}
          </div>
          
          <!-- 详细信息展开 -->
          <Transition name="expand">
            <div v-if="log.expanded" class="log-details">
              <div v-if="log.details" class="log-detail-item">
                <span class="detail-label">详细信息:</span>
                <pre class="detail-content">{{ JSON.stringify(log.details, null, 2) }}</pre>
              </div>
              <div v-if="log.stackTrace" class="log-detail-item">
                <span class="detail-label">堆栈跟踪:</span>
                <pre class="detail-content">{{ log.stackTrace }}</pre>
              </div>
              <div class="log-detail-item">
                <span class="detail-label">完整时间:</span>
                <span class="detail-content">{{ formatFullTime(log.timestamp) }}</span>
              </div>
              <div v-if="log.requestId" class="log-detail-item">
                <span class="detail-label">请求ID:</span>
                <span class="detail-content">{{ log.requestId }}</span>
              </div>
            </div>
          </Transition>
        </div>

        <!-- 空状态 -->
        <div v-if="filteredLogs.length === 0" class="empty-logs">
          <span style="font-size: 64px;">📋</span>
          <h3>暂无日志记录</h3>
          <p>{{ searchKeyword ? '没有找到匹配的日志' : '系统运行日志将显示在这里' }}</p>
        </div>

        <!-- 加载更多 -->
        <div v-if="hasMoreLogs" class="load-more">
          <n-button @click="loadMoreLogs" :loading="loadingMore" block>
            加载更多日志
          </n-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useAppStore } from '../stores/app'
import { useErrorHandler } from '../composables/useErrorHandler'
import {
  NButton,
  NSpace,
  NSelect,
  NDatePicker,
  NInput,
  NTag,
  NSwitch,
  useMessage,
  useDialog
} from 'naive-ui'

const appStore = useAppStore()
const { withErrorHandling, isLoading, error, clearError } = useErrorHandler()
const message = useMessage()
const dialog = useDialog()

// 响应式数据
const loading = ref(false)
const loadingMore = ref(false)
const selectedLevel = ref('')
const dateRange = ref(null)
const searchKeyword = ref('')
const autoScroll = ref(true)
const wordWrap = ref(true)
const showTimestamp = ref(true)
const compactMode = ref(false)
const logContentRef = ref()

// 模拟日志数据（实际应用中应该从后端获取）
const logs = ref([
  {
    id: 1,
    timestamp: new Date(),
    level: 'info',
    message: '应用启动成功',
    source: 'system',
    expanded: false,
    details: null,
    stackTrace: null,
    requestId: null
  },
  {
    id: 2,
    timestamp: new Date(Date.now() - 60000),
    level: 'debug',
    message: '邮件检查服务已启动',
    source: 'email-service',
    expanded: false,
    details: { interval: '5分钟', accounts: 2 },
    stackTrace: null,
    requestId: 'req-001'
  },
  {
    id: 3,
    timestamp: new Date(Date.now() - 120000),
    level: 'warn',
    message: '下载文件夹权限不足',
    source: 'download-service',
    expanded: false,
    details: { path: 'C:\\Downloads\\EmailPDFs', permission: 'read-only' },
    stackTrace: null,
    requestId: 'req-002'
  },
  {
    id: 4,
    timestamp: new Date(Date.now() - 180000),
    level: 'error',
    message: 'IMAP连接失败',
    source: 'email-service',
    expanded: false,
    details: { server: 'imap.gmail.com', port: 993, ssl: true },
    stackTrace: 'Error: Connection timeout\n  at IMAPClient.connect()\n  at EmailService.checkEmails()',
    requestId: 'req-003'
  }
])

const hasMoreLogs = ref(true)

// 日志级别选项
const logLevelOptions = [
  { label: '全部', value: '' },
  { label: '调试', value: 'debug' },
  { label: '信息', value: 'info' },
  { label: '警告', value: 'warn' },
  { label: '错误', value: 'error' }
]

// 计算属性
const filteredLogs = computed(() => {
  let filtered = logs.value || []
  
  // 按级别筛选
  if (selectedLevel.value) {
    filtered = filtered.filter(log => log.level === selectedLevel.value)
  }
  
  // 按关键词搜索
  if (searchKeyword.value) {
    const keyword = searchKeyword.value.toLowerCase()
    filtered = filtered.filter(log => 
      log.message.toLowerCase().includes(keyword) ||
      log.source.toLowerCase().includes(keyword)
    )
  }
  
  // 按日期范围筛选
  if (dateRange.value && dateRange.value[0] && dateRange.value[1]) {
    const [startDate, endDate] = dateRange.value
    filtered = filtered.filter(log => {
      const logDate = new Date(log.timestamp)
      return logDate >= startDate && logDate <= endDate
    })
  }
  
  return filtered.sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())
})

const logStats = computed(() => {
  const allLogs = logs.value || []
  return {
    total: allLogs.length,
    info: allLogs.filter(log => log.level === 'info').length,
    warn: allLogs.filter(log => log.level === 'warn').length,
    error: allLogs.filter(log => log.level === 'error').length,
    debug: allLogs.filter(log => log.level === 'debug').length
  }
})

// 方法
const refreshLogs = async () => {
  loading.value = true
  
  try {
    await withErrorHandling(async () => {
      // 这里应该调用后端API获取日志
      // const result = await appStore.getLogs()
      // logs.value = result
      
      // 模拟刷新
      await new Promise(resolve => setTimeout(resolve, 1000))
      message.success('日志已刷新')
    }, '刷新日志')
  } finally {
    loading.value = false
  }
}

const clearLogs = async () => {
  dialog.warning({
    title: '清空日志',
    content: '确定要清空所有日志记录吗？此操作不可撤销。',
    positiveText: '确定',
    negativeText: '取消',
    onPositiveClick: async () => {
      await withErrorHandling(async () => {
        logs.value = []
        message.success('日志已清空')
      }, '清空日志')
    }
  })
}

const exportLogs = async () => {
  await withErrorHandling(async () => {
    const logData = filteredLogs.value.map(log => ({
      时间: formatFullTime(log.timestamp),
      级别: log.level.toUpperCase(),
      来源: log.source,
      消息: log.message,
      详情: log.details ? JSON.stringify(log.details) : '',
      请求ID: log.requestId || ''
    }))
    
    const csvContent = [
      Object.keys(logData[0]).join(','),
      ...logData.map(row => Object.values(row).map(val => `"${val}"`).join(','))
    ].join('\n')
    
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' })
    const link = document.createElement('a')
    link.href = URL.createObjectURL(blob)
    link.download = `logs-${new Date().toISOString().split('T')[0]}.csv`
    link.click()
    
    message.success('日志已导出')
  }, '导出日志')
}

const loadMoreLogs = async () => {
  loadingMore.value = true
  
  try {
    await withErrorHandling(async () => {
      // 模拟加载更多日志
      await new Promise(resolve => setTimeout(resolve, 1000))
      
      const moreLogs = [
        {
          id: logs.value.length + 1,
          timestamp: new Date(Date.now() - 300000),
          level: 'info',
          message: '定时任务执行成功',
          source: 'scheduler',
          expanded: false,
          details: null,
          stackTrace: null,
          requestId: null
        }
      ]
      
      logs.value.push(...moreLogs)
      
      if (logs.value.length > 50) {
        hasMoreLogs.value = false
      }
    }, '加载更多日志')
  } finally {
    loadingMore.value = false
  }
}

const toggleLogDetails = (index) => {
  const log = filteredLogs.value[index]
  if (log) {
    log.expanded = !log.expanded
  }
}

const copyLogEntry = async (log) => {
  try {
    const logText = `[${formatFullTime(log.timestamp)}] ${log.level.toUpperCase()} ${log.source}: ${log.message}`
    await navigator.clipboard.writeText(logText)
    message.success('日志已复制到剪贴板')
  } catch (error) {
    message.error('复制失败')
  }
}

const getLogLevelClass = (level) => {
  return `log-${level}`
}

const getLogTagType = (level) => {
  const types = {
    debug: 'default',
    info: 'info',
    warn: 'warning',
    error: 'error'
  }
  return types[level] || 'default'
}

const formatTime = (timestamp) => {
  return new Date(timestamp).toLocaleTimeString('zh-CN', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  })
}

const formatFullTime = (timestamp) => {
  return new Date(timestamp).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false
  })
}

// 自动滚动到底部
const scrollToBottom = () => {
  if (autoScroll.value && logContentRef.value) {
    nextTick(() => {
      logContentRef.value.scrollTop = logContentRef.value.scrollHeight
    })
  }
}

// 监听日志变化，自动滚动
watch(() => logs.value.length, () => {
  scrollToBottom()
})

// 生命周期
onMounted(() => {
  refreshLogs()
})
</script>

<style scoped>
.logs {
  padding: 24px;
  height: 100vh;
  display: flex;
  flex-direction: column;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 16px;
}

.page-title {
  font-size: 24px;
  font-weight: 600;
  color: #262626;
  margin: 0 0 8px 0;
}

.page-description {
  color: #666;
  margin: 0;
  font-size: 14px;
}

.log-stats {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.log-container {
  flex: 1;
  border: 1px solid #f0f0f0;
  border-radius: 8px;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.log-controls {
  padding: 12px 16px;
  border-bottom: 1px solid #f0f0f0;
  background: #fafafa;
  border-radius: 8px 8px 0 0;
}

.log-content {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
  background: #fff;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 13px;
  line-height: 1.4;
}

.log-content.word-wrap {
  word-wrap: break-word;
  white-space: pre-wrap;
}

.log-content.compact-mode .log-entry {
  margin-bottom: 4px;
}

.log-entry {
  margin-bottom: 8px;
  padding: 8px 12px;
  border-radius: 4px;
  border-left: 3px solid transparent;
  background: #fafafa;
  transition: all 0.2s;
}

.log-entry:hover {
  background: #f0f0f0;
}

.log-entry.log-debug {
  border-left-color: #d9d9d9;
}

.log-entry.log-info {
  border-left-color: #1890ff;
}

.log-entry.log-warn {
  border-left-color: #faad14;
}

.log-entry.log-error {
  border-left-color: #ff4d4f;
  background: #fff2f0;
}

.log-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
}

.log-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}

.log-time {
  color: #999;
  font-size: 11px;
}

.log-level {
  font-size: 10px;
  font-weight: 600;
}

.log-source {
  color: #666;
  font-size: 11px;
  font-weight: 500;
}

.log-actions {
  display: flex;
  gap: 4px;
  opacity: 0;
  transition: opacity 0.2s;
}

.log-entry:hover .log-actions {
  opacity: 1;
}

.log-message {
  color: #262626;
  word-break: break-all;
}

.log-details {
  margin-top: 8px;
  padding: 8px;
  background: #f8f8f8;
  border-radius: 4px;
  border: 1px solid #e8e8e8;
}

.log-detail-item {
  margin-bottom: 8px;
}

.log-detail-item:last-child {
  margin-bottom: 0;
}

.detail-label {
  font-weight: 600;
  color: #666;
  margin-right: 8px;
}

.detail-content {
  color: #262626;
  font-family: inherit;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
}

.empty-logs {
  text-align: center;
  padding: 80px 24px;
  color: #666;
}

.empty-icon {
  color: #d9d9d9;
  margin-bottom: 16px;
}

.empty-logs h3 {
  font-size: 18px;
  margin: 0 0 8px 0;
}

.empty-logs p {
  margin: 0;
}

.load-more {
  padding: 16px;
  border-top: 1px solid #f0f0f0;
}

.rotated {
  transform: rotate(180deg);
}

/* 展开动画 */
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
  max-height: 300px;
  opacity: 1;
}

/* 响应式设置 */
@media (max-width: 768px) {
  .logs {
    padding: 16px;
  }
  
  .page-header {
    flex-direction: column;
    gap: 16px;
  }
  
  .header-right {
    width: 100%;
  }
  
  .log-stats {
    justify-content: center;
  }
  
  .log-controls {
    padding: 8px 12px;
  }
  
  .log-content {
    font-size: 12px;
  }
}

/* 滚动条样式 */
.log-content::-webkit-scrollbar {
  width: 8px;
}

.log-content::-webkit-scrollbar-track {
  background: #f1f1f1;
}

.log-content::-webkit-scrollbar-thumb {
  background: #c1c1c1;
  border-radius: 4px;
}

.log-content::-webkit-scrollbar-thumb:hover {
  background: #a8a8a8;
}
</style> 
