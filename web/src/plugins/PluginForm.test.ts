import {expect,it,vi} from 'vitest'
import {setupComponent,flush,deferred} from '../test/setup-component'
import {pluginUI} from '../api/plugins'
import PluginForm from './PluginForm.vue'
vi.mock('../api/plugins',()=>({pluginUI:vi.fn()}))
it('disabled forms do not invoke RPC or start a worker and explicitly allow JSON configuration',async()=>{
 const view=setupComponent(PluginForm,{instanceId:'disabled',modelValue:'{}',running:false});await flush()
 expect(pluginUI).not.toHaveBeenCalled();expect(view.state.error).toContain('可直接保存 JSON');expect(view.state.error).toContain('不会自动启用');view.unmount()
})

it('running form exposes pending and failure, with an explicit read-only retry',async()=>{
 const d=deferred<any>();vi.mocked(pluginUI).mockReturnValueOnce(d.promise).mockResolvedValueOnce({content:[]})
 const v=setupComponent(PluginForm,{instanceId:'running',modelValue:'{}',running:true})
 try{expect(v.state.loading).toBe(true);expect(v.state.form).toBeUndefined();d.reject(new Error('fixture'));await flush();expect(v.state.loading).toBe(false);expect(v.state.error).toContain('JSON');await v.state.load();expect(v.state.error).toBe('');expect(v.state.form).toEqual({content:[]})}finally{v.unmount()}
})
