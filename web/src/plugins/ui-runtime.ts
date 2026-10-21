// The only plugin bare import facade. Vite resolves it in the app chunk graph;
// no remote module, runtime rewriting, eval or second Vue runtime is involved.
export { defineComponent, h, ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { ElButton, ElInput, ElSwitch, ElCard, ElAlert, ElTag } from 'element-plus'
import 'element-plus/es/components/button/style/css'
import 'element-plus/es/components/input/style/css'
import 'element-plus/es/components/switch/style/css'
import 'element-plus/es/components/card/style/css'
import 'element-plus/es/components/alert/style/css'
import 'element-plus/es/components/tag/style/css'
import { request, post } from '../api/client'
export const components = Object.freeze({ ElButton, ElInput, ElSwitch, ElCard, ElAlert, ElTag })
export const apiClient = Object.freeze({ request, post })
