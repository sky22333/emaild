import { ref } from 'vue'
import { useMessage, useNotification } from 'naive-ui'

export interface ErrorHandlerOptions {
  showMessage?: boolean
  showNotification?: boolean
  logToConsole?: boolean
  customHandler?: (error: Error) => void
}

export function useErrorHandler() {
  const message = useMessage()
  const notification = useNotification()
  const isLoading = ref(false)
  const error = ref<Error | null>(null)

  const handleError = (
    err: unknown, 
    context = '操作',
    options: ErrorHandlerOptions = {}
  ) => {
    const {
      showMessage = true,
      showNotification = false,
      logToConsole = true,
      customHandler
    } = options

    const errorObj = err instanceof Error ? err : new Error(String(err))
    error.value = errorObj

    if (logToConsole) {
      console.error(`${context}失败:`, errorObj)
    }

    const errorMessage = errorObj.message || '未知错误'

    if (showMessage) {
      message.error(`${context}失败: ${errorMessage}`)
    }

    if (showNotification) {
      notification.error({
        title: `${context}失败`,
        content: errorMessage,
        duration: 5000
      })
    }

    if (customHandler) {
      customHandler(errorObj)
    }

    return errorObj
  }

  const withErrorHandling = async <T>(
    fn: () => Promise<T>,
    context = '操作',
    options: ErrorHandlerOptions = {}
  ): Promise<T | null> => {
    try {
      isLoading.value = true
      error.value = null
      const result = await fn()
      return result
    } catch (err) {
      handleError(err, context, options)
      return null
    } finally {
      isLoading.value = false
    }
  }

  const clearError = () => {
    error.value = null
  }

  return {
    isLoading,
    error,
    handleError,
    withErrorHandling,
    clearError
  }
} 