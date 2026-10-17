import type { Component } from 'vue'
import inventory from '../../../internal/modules/plugins/v2/library.json'
const modules = import.meta.glob<{default:Component}>('../../plugin-ui/*/ui/dist/*.{js,mjs}')
// Entries come exclusively from the immutable inventory, never a server URL.
export async function loadPluginUI(id:string):Promise<Component> {
 const manifest=inventory.manifests.find(m=>m.id===id)
 if (!manifest || manifest.ui.mode!=='vue' || !manifest.permissions.some(p=>p.scope==='ui.trusted')) throw new Error('此插件没有已编译的可信界面')
 const entry=manifest.ui.entry
 if (!entry || !inventory.resources.some(r=>r.id===id&&r.path===entry)) throw new Error('插件界面与构建清单不一致')
 const load=modules[`../../plugin-ui/${id}/${entry}`]
 if(!load) throw new Error('当前镜像未包含此插件界面')
 return (await load()).default
}
