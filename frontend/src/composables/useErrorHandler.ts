import { ref } from 'vue'

export interface ErrorHandlerOptions {
  showMessage?: boolean
  showNotification?: boolean
  logToConsole?: boolean
  customHandler?: (error: Error) => void
}

export function useErrorHandler() {
  const isLoading = ref(false)
  const error = ref<Error | null>(null)

  const handleError = (
    err: unknown, 
    context = '操作',
    options: ErrorHandlerOptions = {}
  ) => {
    const {
      logToConsole = true,
      customHandler
    } = options

    const errorObj = err instanceof Error ? err : new Error(String(err))
    error.value = errorObj

    if (logToConsole) {
      console.error(`${context}失败:`, errorObj)
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