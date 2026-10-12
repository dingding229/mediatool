import { defineConfig } from '@playwright/test'
export default defineConfig({
 testDir:'./e2e',fullyParallel:false,workers:1,
 use:{baseURL:process.env.E2E_BASE_URL??'http://localhost:18084',headless:true,channel:process.env.E2E_BROWSER_CHANNEL||undefined,viewport:{width:1440,height:1000}},
 // Use the lockfile-matched isolated headless browser by default. Actual
 // Chrome checks can opt in with E2E_BROWSER_CHANNEL=chrome.
 // Screenshot/viewport work can be slow on macOS; assertions still use their
 // normal bounded expectations. No retries or force-clicks hide failures.
 reporter:'list',timeout:60000,
})
