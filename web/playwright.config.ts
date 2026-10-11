import { defineConfig } from '@playwright/test'
export default defineConfig({
 testDir:'./e2e',fullyParallel:false,workers:1,
 use:{baseURL:process.env.E2E_BASE_URL??'http://localhost:18084',headless:true,channel:'chrome',viewport:{width:1440,height:1000}},
 reporter:'list',timeout:30000,
})
