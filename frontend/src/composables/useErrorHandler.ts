import { ref, computed, readonly } from 'vue'

// 错误类型枚举
export enum ErrorType {
  NETWORK = 'network',
  AUTHENTICATION = 'authentication', 
  VALIDATION = 'validation',
  PERMISSION = 'permission',
  SERVER = 'server',
  UNKNOWN = 'unknown',
  SERVICE_UNAVAILABLE = 'service_unavailable',
  TIMEOUT = 'timeout'
}

// 错误级别枚举
export enum ErrorLevel {
  INFO = 'info',
  WARNING = 'warning', 
  ERROR = 'error',
  CRITICAL = 'critical'
}

// 错误信息接口
export interface ErrorInfo {
  type: ErrorType
  level: ErrorLevel
  message: string
  originalError?: any
  timestamp: number
  operation?: string
  retryable: boolean
}

// 错误处理配置
interface ErrorHandlerConfig {
  showNotification: boolean
  showMessage: boolean
  logToConsole: boolean
  maxRetries: number
  retryDelay: number
}

const defaultConfig: ErrorHandlerConfig = {
  showNotification: true,
  showMessage: true,
  logToConsole: true,
  maxRetries: 3,
  retryDelay: 1000
}

export function useErrorHandler(config: Partial<ErrorHandlerConfig> = {}) {
  const finalConfig = { ...defaultConfig, ...config }
  
  // 错误状态
  const errors = ref<ErrorInfo[]>([])
  const lastError = ref<ErrorInfo | null>(null)
  const isRetrying = ref(false)
  
  // 计算属性
  const hasErrors = computed(() => errors.value.length > 0)
  const criticalErrors = computed(() => 
    errors.value.filter(error => error.level === ErrorLevel.CRITICAL)
  )
  const recentErrors = computed(() => {
    const oneHourAgo = Date.now() - 60 * 60 * 1000
    return errors.value.filter(error => error.timestamp > oneHourAgo)
  })

  // 错误分类函数
  const classifyError = (error: any, operation?: string): ErrorInfo => {
    const timestamp = Date.now()
    let type = ErrorType.UNKNOWN
    let level = ErrorLevel.ERROR
    let message = '发生未知错误'
    let retryable = false

    if (!error) {
      return {
        type: ErrorType.UNKNOWN,
        level: ErrorLevel.WARNING,
        message: '空错误对象',
        timestamp,
        operation,
        retryable: false
      }
    }

    const errorMessage = error.message || error.toString() || ''
    const errorLower = errorMessage.toLowerCase()

    // 网络错误
    if (errorLower.includes('network') || 
        errorLower.includes('fetch') || 
        errorLower.includes('connection') ||
        errorLower.includes('timeout')) {
      type = ErrorType.NETWORK
      level = ErrorLevel.WARNING
      retryable = true
      
      if (errorLower.includes('timeout')) {
        type = ErrorType.TIMEOUT
        message = '请求超时，请检查网络连接'
      } else if (errorLower.includes('connection')) {
        message = '网络连接失败，请检查网络设置'
      } else {
        message = '网络请求失败，请稍后重试'
      }
    }
    // 认证错误
    else if (errorLower.includes('unauthorized') || 
             errorLower.includes('authentication') ||
             errorLower.includes('login') ||
             errorLower.includes('password')) {
      type = ErrorType.AUTHENTICATION
      level = ErrorLevel.ERROR
      message = '认证失败，请检查账户信息'
      retryable = false
    }
    // 权限错误
    else if (errorLower.includes('forbidden') || 
             errorLower.includes('permission') ||
             errorLower.includes('access denied')) {
      type = ErrorType.PERMISSION
      level = ErrorLevel.ERROR
      message = '权限不足，无法执行此操作'
      retryable = false
    }
    // 验证错误
    else if (errorLower.includes('validation') || 
             errorLower.includes('invalid') ||
             errorLower.includes('required') ||
             errorLower.includes('格式') ||
             errorLower.includes('不能为空')) {
      type = ErrorType.VALIDATION
      level = ErrorLevel.WARNING
      message = errorMessage || '输入数据验证失败'
      retryable = false
    }
    // 服务不可用
    else if (errorLower.includes('service') || 
             errorLower.includes('服务') ||
             errorLower.includes('初始化') ||
             errorLower.includes('未准备')) {
      type = ErrorType.SERVICE_UNAVAILABLE
      level = ErrorLevel.CRITICAL
      message = '服务暂时不可用，请稍后重试'
      retryable = true
    }
    // 服务器错误
    else if (errorLower.includes('server') || 
             errorLower.includes('internal') ||
             errorLower.includes('500') ||
             errorLower.includes('502') ||
             errorLower.includes('503')) {
      type = ErrorType.SERVER
      level = ErrorLevel.ERROR
      message = '服务器错误，请稍后重试'
      retryable = true
    }
    // 其他错误
    else {
      message = errorMessage || '操作失败，请稍后重试'
      retryable = !errorLower.includes('cancelled')
    }

    return {
      type,
      level,
      message,
      originalError: error,
      timestamp,
      operation,
      retryable
    }
  }

  // 处理错误
  const handleError = (error: any, operation?: string): ErrorInfo => {
    const errorInfo = classifyError(error, operation)
    
    // 添加到错误列表
    errors.value.unshift(errorInfo)
    
    // 保持最近100个错误
    if (errors.value.length > 100) {
      errors.value = errors.value.slice(0, 100)
    }
    
    lastError.value = errorInfo
    
    // 控制台日志
    if (finalConfig.logToConsole) {
      const logLevel = errorInfo.level === ErrorLevel.CRITICAL ? 'error' :
                      errorInfo.level === ErrorLevel.ERROR ? 'error' :
                      errorInfo.level === ErrorLevel.WARNING ? 'warn' : 'info'
      
      console[logLevel](`[${operation || 'Unknown'}] ${errorInfo.message}`, {
        type: errorInfo.type,
        level: errorInfo.level,
        originalError: errorInfo.originalError,
        timestamp: new Date(errorInfo.timestamp).toISOString()
      })
    }
    
    // 显示用户通知
    showUserNotification(errorInfo)
    
    return errorInfo
  }

  // 显示用户通知
  const showUserNotification = (errorInfo: ErrorInfo) => {
    const { type, level, message, operation } = errorInfo
    
    // 根据错误级别选择通知方式
    if (level === ErrorLevel.CRITICAL) {
      if (finalConfig.showNotification) {
        console.error(`严重错误 - ${operation ? `${operation}: ` : ''}${message}`)
        // TODO: 集成UI通知组件
      }
    } else if (level === ErrorLevel.ERROR) {
      if (finalConfig.showMessage) {
        console.error(`错误 - ${operation ? `${operation}: ` : ''}${message}`)
        // TODO: 集成UI消息组件
      }
    } else if (level === ErrorLevel.WARNING && finalConfig.showMessage) {
      console.warn(`警告 - ${operation ? `${operation}: ` : ''}${message}`)
      // TODO: 集成UI消息组件
    }
  }

  // 重试机制
  const retryOperation = async <T>(
    operation: () => Promise<T>,
    operationName?: string,
    maxRetries = finalConfig.maxRetries
  ): Promise<T> => {
    let lastError: any
    
    for (let attempt = 0; attempt <= maxRetries; attempt++) {
      try {
        if (attempt > 0) {
          isRetrying.value = true
          // 指数退避延迟
          const delay = finalConfig.retryDelay * Math.pow(2, attempt - 1)
          await new Promise(resolve => setTimeout(resolve, delay))
        }
        
        const result = await operation()
        isRetrying.value = false
        
                 // 如果重试成功，显示成功消息
         if (attempt > 0 && finalConfig.showMessage) {
           console.log(`${operationName || '操作'}重试成功`)
           // TODO: 集成UI成功消息组件
         }
        
        return result
      } catch (error) {
        lastError = error
        const errorInfo = classifyError(error, operationName)
        
        // 如果错误不可重试，直接抛出
        if (!errorInfo.retryable) {
          isRetrying.value = false
          throw error
        }
        
        // 最后一次尝试失败
        if (attempt === maxRetries) {
          isRetrying.value = false
          handleError(error, operationName)
          throw error
        }
        
        // 记录重试日志
        if (finalConfig.logToConsole) {
          console.warn(`[${operationName || 'Operation'}] 尝试 ${attempt + 1}/${maxRetries + 1} 失败，准备重试:`, error)
        }
      }
    }
    
    isRetrying.value = false
    throw lastError
  }

  // 包装异步操作
  const wrapAsync = <T>(
    operation: () => Promise<T>,
    operationName?: string,
    enableRetry = true
  ) => {
    if (enableRetry) {
      return retryOperation(operation, operationName)
    } else {
      return operation().catch(error => {
        handleError(error, operationName)
        throw error
      })
    }
  }

  // 清理错误
  const clearErrors = () => {
    errors.value = []
    lastError.value = null
  }

  // 清理特定类型的错误
  const clearErrorsByType = (type: ErrorType) => {
    errors.value = errors.value.filter(error => error.type !== type)
    if (lastError.value?.type === type) {
      lastError.value = errors.value[0] || null
    }
  }

  // 清理过期错误
  const clearExpiredErrors = (maxAge = 24 * 60 * 60 * 1000) => {
    const cutoff = Date.now() - maxAge
    errors.value = errors.value.filter(error => error.timestamp > cutoff)
    
    if (lastError.value && lastError.value.timestamp <= cutoff) {
      lastError.value = errors.value[0] || null
    }
  }

  // 获取错误统计
  const getErrorStats = () => {
    const stats = {
      total: errors.value.length,
      byType: {} as Record<ErrorType, number>,
      byLevel: {} as Record<ErrorLevel, number>,
      recent: recentErrors.value.length
    }
    
    errors.value.forEach(error => {
      stats.byType[error.type] = (stats.byType[error.type] || 0) + 1
      stats.byLevel[error.level] = (stats.byLevel[error.level] || 0) + 1
    })
    
    return stats
  }

  return {
    // 状态
    errors: readonly(errors),
    lastError: readonly(lastError),
    isRetrying: readonly(isRetrying),
    
    // 计算属性
    hasErrors,
    criticalErrors,
    recentErrors,
    
    // 方法
    handleError,
    retryOperation,
    wrapAsync,
    clearErrors,
    clearErrorsByType,
    clearExpiredErrors,
    getErrorStats,
    
    // 工具方法
    classifyError,
    
    // 类型和枚举
    ErrorType,
    ErrorLevel
  }
} 