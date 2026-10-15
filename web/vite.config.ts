import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import {createHash} from 'node:crypto'
import {readFileSync,writeFileSync} from 'node:fs'
import { fileURLToPath, URL } from 'node:url'
export default defineConfig({
 plugins:[vue(),{name:'safe-pwa-build',writeBundle(options,bundle){const id=createHash('sha256').update(Object.keys(bundle).sort().join('\n')+['sw.js','offline.html','manifest.webmanifest','pwa/icon-192.png','pwa/icon-512.png'].map(name=>readFileSync(fileURLToPath(new URL('./public/'+name,import.meta.url)))).join('')).digest('hex').slice(0,16);const target=fileURLToPath(new URL('../internal/platform/webui/assets/sw.js',import.meta.url));writeFileSync(target,readFileSync(fileURLToPath(new URL('./public/sw.js',import.meta.url)),'utf8').replaceAll('__BUILD_ID__',id))}}],
 server:{port:5173,proxy:{'/api':{target:'http://127.0.0.1:8080',changeOrigin:false},'/readyz':'http://127.0.0.1:8080'}},
 build:{outDir:fileURLToPath(new URL('../internal/platform/webui/assets',import.meta.url)),emptyOutDir:false,chunkSizeWarningLimit:700},
})
