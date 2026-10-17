import {describe,it,expect,vi}from'vitest'
import {createApp,h}from'vue'
import {renderToString}from'vue/server-renderer'
import PluginJson from './PluginJson'
import {pluginUIBuildGuard}from'./build-guard'
describe('plugin JSON contracts',()=>{
 it('escapes text and safely placeholders unknown components without raw props',async()=>{const html=await renderToString(createApp({render:()=>h(PluginJson,{nodes:[{component:'Text',props:{text:'<script>bad()</script>',innerHTML:'<img src=x onerror=bad()>'}},{component:'script',props:{src:'https://remote.invalid/ui.js'}}]})}));expect(html).toContain('&lt;script&gt;');expect(html).toContain('不支持的组件');expect(html).not.toContain('<script>');expect(html).not.toContain('remote.invalid')})
 it('conditions and flat safe models are not evaluated as expressions',async()=>{const html=await renderToString(createApp({render:()=>h(PluginJson,{model:{enabled:false},nodes:[{component:'Text',props:{text:'hidden'},when:{model:'enabled',equals:true}},{component:'VTextField',model:'__proto__',props:{label:'安全字段',onClick:'bad()',href:'javascript:bad()'}}]})}));expect(html).not.toContain('hidden');expect(html).toContain('disabled');expect(html).not.toContain('bad()')})
 it('bounds total nodes/depth',async()=>{const html=await renderToString(createApp({render:()=>h(PluginJson,{nodes:Array.from({length:257},()=>({component:'Text',props:{text:'safe'}}))})}));expect(html).toContain('安全上限')})
 it('build only permits same-plugin relative modules or shared facade',()=>{const p=pluginUIBuildGuard() as any;const fn=p.resolveId;expect(()=>fn.call({},'https://remote.invalid/a.js','/web/plugin-ui/hello-go/ui/dist/index.js')).toThrow();expect(()=>fn.call({},'vue','/web/plugin-ui/hello-go/ui/dist/index.js')).toThrow();expect(()=>fn.call({},'../../../../other/ui.js','/web/plugin-ui/hello-go/ui/dist/index.js')).toThrow();expect(fn.call({},'@mediatool/plugin-ui','/web/plugin-ui/hello-go/ui/dist/index.js')).toBeUndefined();expect(fn.call({},'./other.js','/web/plugin-ui/hello-go/ui/dist/index.js')).toBeUndefined()})
})

it('model updates emit plain config values and honor readonly/prototype guards',()=>{
 const emit=vi.fn(),render=(PluginJson as any).setup({nodes:[{component:'VTextField',model:'greeting',props:{label:'问候语'}}],model:{scheduled:true},readonly:false},{emit})
 const input=render().children[0].children[1]
 input.props['onUpdate:modelValue']('Owned')
 expect(emit).toHaveBeenCalledWith('update:model',{scheduled:true,greeting:'Owned'})
})

it('renders the Go/Vue shared form contract with bound defaults and conditions',async()=>{
 const fixture=(await import('../../../internal/platform/httpapi/testdata/contracts/plugin-v2-ui.json')).default
 const html=await renderToString(createApp({render:()=>h(PluginJson,{nodes:fixture.form.content,model:fixture.form.defaults})}))
 expect(html).toContain('问候语');expect(html).toContain('定时服务已开启');const render=(PluginJson as any).setup({nodes:fixture.form.content,model:fixture.form.defaults,readonly:false},{emit:vi.fn()});expect(render().children[0].children[1].props.modelValue).toBe(fixture.form.defaults.greeting)
})
