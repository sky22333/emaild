import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'

export default defineConfig({
  plugins: [
    vue(),
    AutoImport({
      imports: [
        'vue',
        'vue-router',
        'pinia',
        {
          'naive-ui': [
            'useDialog',
            'useMessage',
            'useNotification',
            'useLoadingBar'
          ]
        }
      ],
      dts: false // 禁用类型声明文件生成
    }),
    Components({
      resolvers: [NaiveUiResolver()],
      dts: false // 禁用类型声明文件生成
    })
  ],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
      '#': resolve(__dirname, 'types')
    }
  },
  server: {
    port: 34116,
    host: '127.0.0.1',
    strictPort: true,
    cors: true
  },
  build: {
    outDir: 'dist',
    assetsDir: 'assets',
    minify: 'esbuild', // 使用更快的esbuild
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ['vue', 'vue-router', 'pinia'],
          ui: ['naive-ui'],
          charts: ['echarts', 'vue-echarts']
        }
      }
    },
    chunkSizeWarningLimit: 1000,
    reportCompressedSize: false // 禁用压缩大小报告以加快构建
  },
  optimizeDeps: {
    include: [
      'vue',
      'vue-router',
      'pinia',
      'naive-ui',
      'echarts',
      'vue-echarts'
    ]
  }
}) 