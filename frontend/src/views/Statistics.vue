<template>
  <div class="statistics">
    <!-- 页面标题 -->
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">统计分析</h1>
        <p class="page-description">查看下载统计、性能指标和使用趋势</p>
      </div>
      <div class="header-right">
        <n-date-picker
          v-model:value="dateRange"
          type="daterange"
          clearable
          @update:value="handleDateRangeChange"
        />
        <n-button @click="refreshData" :loading="loading">
          <template #icon>
            <span style="font-size: 16px;">🔄</span>
          </template>
          刷新数据
        </n-button>
      </div>
    </div>

    <!-- 总览卡片 -->
    <div class="overview-cards">
      <n-card class="overview-card">
        <n-statistic
          label="总下载量"
          :value="statistics.totalDownloads"
          class="statistic-item"
        >
          <template #prefix>
            <span style="font-size: 24px; color: #1890ff;">⬇️</span>
          </template>
        </n-statistic>
        <div class="statistic-trend">
          <span :class="getTrendClass(statistics.downloadsTrend)">
            <span style="font-size: 14px;">{{ getTrendIcon(statistics.downloadsTrend) }}</span>
            {{ Math.abs(statistics.downloadsTrend) }}%
          </span>
          <span class="trend-period">较上周</span>
        </div>
      </n-card>

      <n-card class="overview-card">
        <n-statistic
          label="成功率"
          :value="statistics.successRate"
          suffix="%"
          class="statistic-item"
        >
          <template #prefix>
            <span style="font-size: 24px; color: #52c41a;">✅</span>
          </template>
        </n-statistic>
        <div class="statistic-trend">
          <span :class="getTrendClass(statistics.successRateTrend)">
            <span style="font-size: 14px;">{{ getTrendIcon(statistics.successRateTrend) }}</span>
            {{ Math.abs(statistics.successRateTrend) }}%
          </span>
          <span class="trend-period">较上周</span>
        </div>
      </n-card>

      <n-card class="overview-card">
        <n-statistic
          label="总存储"
          :value="formatFileSize(statistics.totalSize)"
          class="statistic-item"
        >
          <template #prefix>
            <span style="font-size: 24px; color: #722ed1;">💾</span>
          </template>
        </n-statistic>
        <div class="statistic-trend">
          <span :class="getTrendClass(statistics.storageTrend)">
            <span style="font-size: 14px;">{{ getTrendIcon(statistics.storageTrend) }}</span>
            {{ Math.abs(statistics.storageTrend) }}%
          </span>
          <span class="trend-period">较上周</span>
        </div>
      </n-card>

      <n-card class="overview-card">
        <n-statistic
          label="活跃邮箱"
          :value="statistics.activeEmails"
          class="statistic-item"
        >
          <template #prefix>
            <span style="font-size: 24px; color: #fa8c16;">📧</span>
          </template>
        </n-statistic>
        <div class="statistic-trend">
          <span :class="getTrendClass(statistics.activeEmailsTrend)">
            <span style="font-size: 14px;">{{ getTrendIcon(statistics.activeEmailsTrend) }}</span>
            {{ Math.abs(statistics.activeEmailsTrend) }}%
          </span>
          <span class="trend-period">较上周</span>
        </div>
      </n-card>
    </div>

    <!-- 图表区域 -->
    <div class="charts-grid">
      <!-- 下载趋势图 -->
      <n-card title="下载趋势" class="chart-card">
        <template #header-extra>
          <n-select
            v-model:value="downloadChartType"
            :options="chartTypeOptions"
            size="small"
            style="width: 120px"
          />
        </template>
        <div class="chart-container" ref="downloadChartRef">
          <div class="chart-placeholder">
            <span style="font-size: 48px; color: #d9d9d9;">📈</span>
            <p>下载趋势图表</p>
            <p class="chart-desc">显示{{ dateRangeText }}的下载趋势</p>
          </div>
        </div>
      </n-card>

      <!-- 成功率分析 -->
      <n-card title="成功率分析" class="chart-card">
        <div class="chart-container" ref="successChartRef">
          <div class="chart-placeholder">
            <span style="font-size: 48px; color: #d9d9d9;">🍩</span>
            <p>成功率分析</p>
            <p class="chart-desc">按邮箱分析下载成功率</p>
          </div>
        </div>
      </n-card>

      <!-- 文件类型分布 -->
      <n-card title="文件大小分布" class="chart-card">
        <div class="chart-container" ref="sizeChartRef">
          <div class="chart-placeholder">
            <span style="font-size: 48px; color: #d9d9d9;">📊</span>
            <p>文件大小分布</p>
            <p class="chart-desc">不同大小范围的文件数量</p>
          </div>
        </div>
      </n-card>

      <!-- 邮箱活跃度 -->
      <n-card title="邮箱活跃度" class="chart-card">
        <div class="chart-container" ref="emailChartRef">
          <div class="chart-placeholder">
            <span style="font-size: 48px; color: #d9d9d9;">📧</span>
            <p>邮箱活跃度排行</p>
            <p class="chart-desc">按下载量排序的邮箱活跃度</p>
          </div>
        </div>
      </n-card>
    </div>

    <!-- 详细统计表格 -->
    <n-card title="详细统计" class="table-card">
      <template #header-extra>
        <n-space>
          <n-select
            v-model:value="tableGroupBy"
            :options="groupByOptions"
            size="small"
            style="width: 120px"
          />
          <n-button size="small" @click="exportData">
            <template #icon>
              <span style="font-size: 16px;">📥</span>
            </template>
            导出数据
          </n-button>
        </n-space>
      </template>
      
      <n-data-table
        :columns="tableColumns"
        :data="tableData"
        :pagination="tablePagination"
        :loading="tableLoading"
      />
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useAppStore } from '@/stores/app'
import {
  NCard,
  NStatistic,
  NDatePicker,
  NButton,
  NSelect,
  NSpace,
  NDataTable
} from 'naive-ui'

const appStore = useAppStore()

// 响应式数据
const loading = ref(false)
const tableLoading = ref(false)
const dateRange = ref<[number, number] | null>(null)
const downloadChartType = ref('line')
const tableGroupBy = ref('date')

// 图表引用
const downloadChartRef = ref()
const successChartRef = ref()
const sizeChartRef = ref()
const emailChartRef = ref()

// 统计数据
const statistics = ref({
  totalDownloads: 1250,
  downloadsTrend: 12.5,
  successRate: 94.2,
  successRateTrend: 2.1,
  totalSize: 2147483648, // 2GB
  storageTrend: 8.3,
  activeEmails: 5,
  activeEmailsTrend: 0
})

// 计算属性
const dateRangeText = computed(() => {
  if (!dateRange.value) return '全部时间'
  const [start, end] = dateRange.value
  const startDate = new Date(start).toLocaleDateString()
  const endDate = new Date(end).toLocaleDateString()
  return `${startDate} - ${endDate}`
})

// 选项配置
const chartTypeOptions = [
  { label: '折线图', value: 'line' },
  { label: '柱状图', value: 'bar' },
  { label: '面积图', value: 'area' }
]

const groupByOptions = [
  { label: '按日期', value: 'date' },
  { label: '按邮箱', value: 'email' },
  { label: '按文件大小', value: 'size' }
]

// 表格配置
const tableColumns = [
  { title: '日期', key: 'date', width: 120 },
  { title: '下载数量', key: 'downloads', width: 100 },
  { title: '成功数量', key: 'success', width: 100 },
  { title: '失败数量', key: 'failed', width: 100 },
  { title: '成功率', key: 'successRate', width: 100 },
  { title: '总大小', key: 'totalSize', width: 120 }
]

const tableData = ref([
  {
    date: '2024-01-15',
    downloads: 45,
    success: 42,
    failed: 3,
    successRate: '93.3%',
    totalSize: '125MB'
  },
  {
    date: '2024-01-14',
    downloads: 38,
    success: 36,
    failed: 2,
    successRate: '94.7%',
    totalSize: '98MB'
  }
])

const tablePagination = {
  pageSize: 10
}

// 方法
const getTrendClass = (trend: number) => {
  return trend >= 0 ? 'trend-up' : 'trend-down'
}

const getTrendIcon = (trend: number) => {
  return trend >= 0 ? '📈' : '📉'
}

const formatFileSize = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const handleDateRangeChange = (value: [number, number] | null) => {
  dateRange.value = value
  refreshData()
}

const refreshData = async () => {
  try {
    loading.value = true
    // 这里调用后端API获取统计数据
    await new Promise(resolve => setTimeout(resolve, 1000)) // 模拟API调用
    await withErrorHandling(async () => {
    await appStore.loadStatistics()
  }, '刷新统计数据')
  } catch (error) {
    console.error('刷新数据失败:', error)
  } finally {
    loading.value = false
  }
}

const exportData = () => {
  // TODO: 实现统计数据导出功能
}

// 生命周期
onMounted(() => {
  refreshData()
})
</script>

<style scoped>
.statistics {
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

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.overview-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}

.overview-card {
  transition: all 0.3s;
}

.overview-card:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.statistic-item {
  margin-bottom: 12px;
}

.statistic-trend {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
}

.trend-up {
  color: #52c41a;
}

.trend-down {
  color: #ff4d4f;
}

.trend-period {
  color: #999;
}

.charts-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}

.chart-card {
  min-height: 400px;
}

.chart-container {
  height: 300px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.chart-placeholder {
  text-align: center;
  color: #999;
}

.chart-placeholder p {
  margin: 8px 0;
}

.chart-desc {
  font-size: 12px;
}

.table-card {
  margin-bottom: 24px;
}

@media (max-width: 768px) {
  .statistics {
    padding: 16px;
  }
  
  .page-header {
    flex-direction: column;
    gap: 16px;
  }
  
  .overview-cards {
    grid-template-columns: 1fr;
  }
  
  .charts-grid {
    grid-template-columns: 1fr;
  }
}
</style> 
