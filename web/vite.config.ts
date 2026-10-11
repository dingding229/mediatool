import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'
export default defineConfig({
 plugins:[vue()],
 server:{port:5173,proxy:{'/api':{target:'http://127.0.0.1:8080',changeOrigin:false},'/readyz':'http://127.0.0.1:8080'}},
 build:{outDir:fileURLToPath(new URL('../internal/platform/webui/assets',import.meta.url)),emptyOutDir:true,chunkSizeWarningLimit:700},
})
