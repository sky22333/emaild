<template>
  <div class="email-accounts">
    <!-- 页面标题 -->
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">邮箱管理</h1>
        <p class="page-description">管理多个邮箱账户，配置IMAP连接</p>
      </div>
      <div class="header-right">
        <n-button type="primary" @click="showAddModal = true">
          <template #icon>
            <span style="font-size: 16px;">➕</span>
          </template>
          添加邮箱
        </n-button>
      </div>
    </div>

    <!-- 邮箱列表 -->
    <div class="email-list">
      <n-card
        v-for="account in emailAccounts"
        :key="account.id"
        class="email-card"
        :class="{ active: account.is_active }"
      >
        <div class="email-card-header">
          <div class="email-info">
            <div class="email-avatar">
              <span style="font-size: 24px;">📧</span>
            </div>
            <div class="email-details">
              <div class="email-address">{{ account.email }}</div>
              <div class="email-provider">{{ account.imap_server }}:{{ account.imap_port }}</div>
            </div>
          </div>
          
          <div class="email-actions">
            <n-tag :type="account.is_active ? 'success' : 'default'" size="small">
              {{ account.is_active ? '已启用' : '已禁用' }}
            </n-tag>
            
            <n-dropdown
              :options="getAccountActions(account)"
              @select="handleAccountAction($event, account)"
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

        <div class="email-stats">
          <div class="stat-item">
            <span class="stat-label">今日邮件</span>
            <span class="stat-value">{{ account.stats.todayEmails }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">总下载量</span>
            <span class="stat-value">{{ account.stats.totalDownloads }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">存储大小</span>
            <span class="stat-value">{{ formatFileSize(account.stats.totalSize) }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">成功率</span>
            <span class="stat-value">{{ account.stats.successRate }}%</span>
          </div>
        </div>

        <!-- 展开的详细信息 -->
        <Transition name="expand">
          <div v-if="account.expanded" class="email-details-expanded">
            <n-divider />
            <div class="detail-grid">
              <div class="detail-section">
                <h4>连接配置</h4>
                <div class="detail-item">
                  <span class="label">IMAP服务器:</span>
                  <span class="value">{{ account.imap_server }}:{{ account.imap_port }}</span>
                </div>
                <div class="detail-item">
                  <span class="label">使用SSL:</span>
                  <span class="value">{{ account.use_ssl ? '是' : '否' }}</span>
                </div>
                <div class="detail-item">
                  <span class="label">检查间隔:</span>
                  <span class="value">{{ account.check_interval }}分钟</span>
                </div>
              </div>
              
              <div class="detail-section">
                <h4>下载设置</h4>
                <div class="detail-item">
                  <span class="label">下载目录:</span>
                  <span class="value">{{ account.download_path || '默认目录' }}</span>
                </div>
                <div class="detail-item">
                  <span class="label">文件过滤:</span>
                  <span class="value">{{ account.file_filter || '*.pdf' }}</span>
                </div>
                <div class="detail-item">
                  <span class="label">最大文件大小:</span>
                  <span class="value">{{ formatFileSize(account.max_file_size) }}</span>
                </div>
              </div>
            </div>
            
            <div class="detail-actions">
              <n-button size="small" @click="testConnection(account)" :loading="account.testing">
                <template #icon>
                  <span style="font-size: 16px;">🔗</span>
                </template>
                测试连接
              </n-button>
              <n-button size="small" @click="checkEmails(account)" :loading="account.checking">
                <template #icon>
                  <span style="font-size: 16px;">🔄</span>
                </template>
                检查邮件
              </n-button>
              <n-button size="small" @click="editAccount(account)">
                <template #icon>
                  <span style="font-size: 16px;">✏</span>
                </template>
                编辑配置
              </n-button>
            </div>
          </div>
        </Transition>

        <div class="expand-toggle" @click="toggleExpanded(account)">
          <span style="font-size: 16px;">{{ account.expanded ? '▲' : '▼' }}</span>
        </div>
      </n-card>

      <!-- 空状态 -->
      <div v-if="emailAccounts.length === 0" class="empty-state">
        <span style="font-size: 64px;">📭</span>
        <h3>暂无邮箱账户</h3>
        <p>添加邮箱账户以开始自动下载PDF文件</p>
        <n-button type="primary" @click="showAddModal = true">
          添加邮箱
        </n-button>
      </div>
    </div>

    <!-- 添加/编辑邮箱模态框 -->
    <n-modal v-model:show="showAddModal" preset="dialog" title="添加邮箱账户">
      <n-form :model="currentAccount" :rules="accountRules" ref="accountFormRef">
        <n-form-item label="邮箱地址" path="email">
          <n-input v-model:value="currentAccount.email" placeholder="请输入邮箱地址" />
        </n-form-item>
        
        <n-form-item label="邮箱密码" path="password">
          <n-input v-model:value="currentAccount.password" type="password" placeholder="请输入邮箱密码或授权码" />
        </n-form-item>
        
        <n-form-item label="IMAP服务器" path="imap_server">
          <n-input v-model:value="currentAccount.imap_server" placeholder="如: imap.qq.com" />
        </n-form-item>
        
        <n-form-item label="IMAP端口" path="imap_port">
          <n-input-number v-model:value="currentAccount.imap_port" :min="1" :max="65535" />
        </n-form-item>
        
        <n-form-item label="使用SSL">
          <n-switch v-model:value="currentAccount.use_ssl" />
        </n-form-item>
        
        <n-form-item label="启用账户">
          <n-switch v-model:value="currentAccount.is_active" />
        </n-form-item>
      </n-form>
      
      <template #action>
        <n-space>
          <n-button @click="testConnection" :loading="testing">测试连接</n-button>
          <n-button @click="showAddModal = false">取消</n-button>
          <n-button type="primary" @click="saveAccount" :loading="saving">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- 删除确认对话框 -->
    <n-modal v-model:show="showDeleteDialog" preset="dialog" type="warning">
      <template #header>删除邮箱账户</template>
      <div>
        确定要删除邮箱账户<strong>{{ deletingAccount?.email }}</strong>吗？
        <br>
        此操作不可撤销，但已下载的文件不会被删除。
      </div>
      <template #action>
        <n-button @click="showDeleteDialog = false">取消</n-button>
        <n-button type="error" @click="confirmDelete" :loading="deleting">
          删除
        </n-button>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, reactive, h } from 'vue'
import { useErrorHandler } from '../composables/useErrorHandler'
import {
  NButton,
  NCard,
  NTag,
  NDropdown,
  NModal,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSwitch,
  NSpace,
  useMessage,
  useDialog
} from 'naive-ui'
import { useAppStore } from '../stores/app'

const appStore = useAppStore()
const { withErrorHandling, isLoading, error, clearError } = useErrorHandler()
const message = useMessage()
const dialog = useDialog()

// 响应式数据
const showAddModal = ref(false)
const showDeleteDialog = ref(false)
const editingAccount = ref<any>(null)
const deletingAccount = ref<any>(null)
const testing = ref(false)
const saving = ref(false)
const deleting = ref(false)

// 表单相关
const accountFormRef = ref()
const currentAccount = ref({
  id: 0,
  name: '',
  email: '',
  password: '',
  imap_server: '',
  imap_port: 993,
  use_ssl: true,
  is_active: true,
  created_at: '',
  updated_at: ''
})

// 表单验证规则
const accountRules = {
  email: [
    { required: true, message: '请输入邮箱地址' },
    { type: 'email', message: '请输入有效的邮箱地址' }
  ],
  password: [
    { required: true, message: '请输入邮箱密码或授权码' }
  ],
  imap_server: [
    { required: true, message: '请输入IMAP服务器地址' }
  ],
  imap_port: [
    { required: true, type: 'number', message: '请输入有效的端口号' }
  ]
}

// 邮箱账户列表
const emailAccounts = computed(() => {
  const accounts = appStore.emailAccounts || []
  return accounts.map(account => ({
    ...account,
    expanded: account.expanded || false,
    testing: account.testing || false,
    checking: account.checking || false,
    stats: account.stats || {
      todayEmails: 0,
      totalDownloads: 0,
      totalSize: 0,
      successRate: 0
    }
  }))
})

// 组件挂载
onMounted(() => {
  loadEmailAccounts()
})

// 获取账户操作菜单
const getAccountActions = (account: any) => {
  const actions = []
  
  if (account.is_active) {
    actions.push({ label: '禁用账户', key: 'disable' })
  } else {
    actions.push({ label: '启用账户', key: 'enable' })
  }
  
  actions.push(
    { label: '测试连接', key: 'test' },
    { label: '检查邮件', key: 'check' },
    { label: '编辑账户', key: 'edit' },
    { label: '删除账户', key: 'delete' }
  )
  
  return actions
}

// 处理账户操作
const handleAccountAction = async (key: string, account: any) => {
  switch (key) {
    case 'enable':
    case 'disable':
      await toggleAccount(account)
      break
    case 'test':
      await testConnection(account)
      break
    case 'check':
      await checkEmails(account)
      break
    case 'edit':
      editAccount(account)
      break
    case 'delete':
      deleteAccount(account)
      break
  }
}

// 切换账户启用状态
const toggleAccount = async (account: any) => {
  await withErrorHandling(async () => {
    const updatedAccount = { ...account, is_active: !account.is_active }
    await appStore.updateEmailAccount(updatedAccount)
    message.success(updatedAccount.is_active ? '账户已启用' : '账户已禁用')
  }, '切换账户状态')
}

// 测试账户连接
const testConnection = async (account: any) => {
  testing.value = true
  
  try {
    await withErrorHandling(async () => {
      await appStore.testEmailConnection(account.id)
      message.success('连接测试成功')
    }, '测试邮箱连接')
  } finally {
    testing.value = false
  }
}

// 检查邮件
const checkEmails = async (account: any) => {
  account.checking = true
  
  try {
    await withErrorHandling(async () => {
      const result = await appStore.checkSingleEmail(account.id)
      message.success(`检查完成：发现 ${result.new_emails} 封新邮件，${result.pdfs_found} 个PDF文件`)
    }, '检查邮件')
  } finally {
    account.checking = false
  }
}

// 编辑账户
const editAccount = (account: any) => {
  editingAccount.value = account
  Object.assign(currentAccount.value, {
    email: account.email,
    password: '', // 出于安全考虑，不显示密码
    imap_server: account.imap_server,
    imap_port: account.imap_port,
    use_ssl: account.use_ssl,
    is_active: account.is_active,
    connectionStatus: account.connectionStatus,
    lastCheck: account.lastCheck,
    processedCount: account.processedCount
  })
  showAddModal.value = true
}

// 删除账户
const deleteAccount = (account: any) => {
  deletingAccount.value = account
  showDeleteDialog.value = true
}

// 确认删除
const confirmDelete = async () => {
  if (!deletingAccount.value) return
  
  deleting.value = true
  
  try {
    await withErrorHandling(async () => {
      await appStore.deleteEmailAccount(deletingAccount.value.id)
      message.success('账户已删除')
      showDeleteDialog.value = false
      deletingAccount.value = null
    }, '删除邮箱账户')
  } finally {
    deleting.value = false
  }
}

// 切换展开状态
const toggleExpanded = (account: any) => {
  account.expanded = !account.expanded
}

// 保存账户
const saveAccount = async () => {
  if (!accountFormRef.value) return
  
  try {
    await accountFormRef.value.validate()
    saving.value = true
    
    if (editingAccount.value) {
      await appStore.updateEmailAccount(editingAccount.value.id, currentAccount.value)
      message.success('账户已更新')
    } else {
      await appStore.addEmailAccount(currentAccount.value)
      message.success('账户已添加')
    }
    
    showAddModal.value = false
    resetForm()
    loadEmailAccounts()
  } catch (error) {
    message.error('保存失败')
  } finally {
    saving.value = false
  }
}

const resetForm = () => {
  currentAccount.value = {
    id: 0,
    name: '',
    email: '',
    password: '',
    imap_server: '',
    imap_port: 993,
    use_ssl: true,
    is_active: true,
    created_at: '',
    updated_at: ''
  }
}

const loadEmailAccounts = async () => {
  await withErrorHandling(async () => {
    await appStore.loadEmailAccounts()
  }, '加载邮箱账户')
}

const formatFileSize = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}
</script>

<style scoped>
.email-accounts {
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

.header-left {
  flex: 1;
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

.email-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.email-card {
  transition: all 0.3s;
  cursor: pointer;
}

.email-card:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.email-card.active {
  border-left: 4px solid #52c41a;
}

.email-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.email-info {
  display: flex;
  align-items: center;
  gap: 12px;
}

.email-avatar {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: #f0f0f0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.email-details {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.email-address {
  font-size: 16px;
  font-weight: 500;
  color: #262626;
}

.email-provider {
  font-size: 12px;
  color: #666;
}

.email-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.email-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: 16px;
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid #f0f0f0;
}

.stat-item {
  text-align: center;
}

.stat-label {
  display: block;
  font-size: 12px;
  color: #666;
  margin-bottom: 4px;
}

.stat-value {
  display: block;
  font-size: 18px;
  font-weight: 600;
  color: #262626;
}

.email-details-expanded {
  margin-top: 16px;
}

.detail-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 24px;
  margin-bottom: 16px;
}

.detail-section h4 {
  font-size: 14px;
  font-weight: 600;
  margin: 0 0 12px 0;
  color: #262626;
}

.detail-item {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
  font-size: 13px;
}

.detail-item .label {
  color: #666;
}

.detail-item .value {
  font-weight: 500;
  color: #262626;
}

.detail-actions {
  display: flex;
  gap: 8px;
}

.expand-toggle {
  position: absolute;
  bottom: 8px;
  right: 50%;
  transform: translateX(50%);
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: #f0f0f0;
  transition: all 0.3s;
  cursor: pointer;
}

.expand-toggle:hover {
  background: #d9d9d9;
}

.expand-toggle .n-icon {
  transition: transform 0.3s;
}

.expand-toggle .n-icon.rotated {
  transform: rotate(180deg);
}

.empty-state {
  text-align: center;
  padding: 60px 0;
  color: #999;
}

.empty-state h3 {
  margin: 16px 0 8px 0;
  color: #262626;
}

.empty-state p {
  margin: 0 0 24px 0;
}

.email-form {
  max-height: 500px;
  overflow-y: auto;
  padding-right: 8px;
}

.dialog-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
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
  max-height: 200px;
  opacity: 1;
}

/* 响应式设置 */
@media (max-width: 768px) {
  .email-accounts {
    padding: 16px;
  }
  
  .page-header {
    flex-direction: column;
    gap: 16px;
  }
  
  .email-card-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 16px;
  }
  
  .email-actions {
    align-self: flex-end;
  }
  
  .email-stats {
    grid-template-columns: repeat(2, 1fr);
  }
  
  .detail-grid {
    grid-template-columns: 1fr;
    gap: 16px;
  }
}

/* 滚动条样式 */
.email-form::-webkit-scrollbar {
  width: 4px;
}

.email-form::-webkit-scrollbar-track {
  background: #f1f1f1;
}

.email-form::-webkit-scrollbar-thumb {
  background: #c1c1c1;
  border-radius: 2px;
}
</style> 
