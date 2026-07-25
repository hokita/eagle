import { defineConfig } from '@playwright/test'

const API_PORT = 8080
const FRONTEND_PORT = 3000

export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['html', { open: 'never' }]],
  use: {
    baseURL: `http://localhost:${FRONTEND_PORT}`,
    trace: 'retain-on-failure',
  },
  webServer: [
    {
      command: 'go run ./cmd/e2eserver',
      cwd: '../api',
      url: `http://localhost:${API_PORT}/api/liveness`,
      reuseExistingServer: false,
      env: {
        GOOGLE_CLOUD_PROJECT: 'eagle-test',
        ALLOWED_EMAIL: 'e2e-test@example.com',
        FRONTEND_URL: `http://localhost:${FRONTEND_PORT}`,
        PORT: String(API_PORT),
      },
    },
    {
      command: `npm run build && npx serve out -l ${FRONTEND_PORT} -s`,
      cwd: '../fe',
      url: `http://localhost:${FRONTEND_PORT}`,
      reuseExistingServer: false,
      env: {
        NEXT_PUBLIC_API_URL: `http://localhost:${API_PORT}`,
        NEXT_PUBLIC_FIREBASE_API_KEY: 'e2e-fake-key',
        NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN: 'eagle-test.firebaseapp.com',
        NEXT_PUBLIC_FIREBASE_PROJECT_ID: 'eagle-test',
        NEXT_PUBLIC_FIREBASE_AUTH_EMULATOR_HOST: 'http://localhost:9099',
      },
    },
  ],
})
