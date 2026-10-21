import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
export default defineConfig({plugins:[vue(),{name:'node-unit-component-css',enforce:'pre',resolveId(id){if(/^element-plus\/es\/components\/[a-z-]+\/style\/css$/.test(id))return '\0unit-css:'+id},load(id){if(id.startsWith('\0unit-css:'))return 'export default {}'}}],test:{include:['src/**/*.test.ts'],environment:'node'}})
