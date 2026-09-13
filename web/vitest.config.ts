import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const rootDir = dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': resolve(rootDir, 'src'),
    },
  },
  test: {
    environment: 'happy-dom',
    setupFiles: ['./vitest.setup.ts'],
    globals: true,
    include: [
      'src/lib/__tests__/status-query.test.tsx',
      'src/features/users/components/dialogs/__tests__/user-binding-dialog.test.tsx',
      'src/features/pricing/lib/__tests__/time-rule-expr.test.ts',
      'src/features/pricing/components/__tests__/model-perf-badge.test.tsx',
      'src/features/usage-logs/lib/__tests__/csv.test.ts',
    ],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html'],
    },
  },
})
