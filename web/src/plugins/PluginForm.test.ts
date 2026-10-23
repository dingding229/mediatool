import {expect,it,vi} from 'vitest'
import {setupComponent,flush} from '../test/setup-component'
import PluginForm from './PluginForm.vue'
import {staticPluginForm} from '../api/plugins'
import {validateForm} from './form-validation'
import inventory from '../../../internal/modules/plugins/v2/library.json'
import PluginJson from './PluginJson'
const nodes=(items:any[]):any[]=>items.flatMap(n=>[n,...nodes(n.children||[])])
it('stopped and running configuration uses same-pin static metadata without HTTP or worker invocation',async()=>{
 const fetcher=vi.fn();vi.stubGlobal('fetch',fetcher)
 try{const v=setupComponent(PluginForm,{pluginId:'library-refresh-notify',modelValue:'{"intervalSeconds":61,"preserved":"owned"}'});await flush();expect(v.state.model.intervalSeconds).toBe(61);expect(v.state.error).toBe('');expect(v.state.form).toEqual(staticPluginForm('library-refresh-notify'));expect(fetcher).not.toHaveBeenCalled();v.unmount()}finally{vi.unstubAllGlobals()}
})
it('every numeric form control accepts both boundaries and off-preset legal values, emits numbers and retains unrelated saved fields',()=>{
 for(const manifest of inventory.manifests){const form=staticPluginForm(manifest.id)!;expect(form).toBeTruthy();for(const field of nodes(form.content).filter(n=>n.component==='VNumberInput')){
  const p=field.props;for(const value of [p.min,p.max,p.min+(p.stepStrictly?1:.001)]){
   const model={...form.defaults,[field.model]:value,unknownSavedKey:'preserved'};expect(validateForm(form.content,model)).toBe('')
   const emit=vi.fn(),render=(PluginJson as any).setup({nodes:[field],model,readonly:false},{emit}),input=render().children[0].children[1];expect(input.props.min).toBe(p.min);expect(input.props.max).toBe(p.max);expect(input.props.step).toBe(p.step);input.props['onUpdate:modelValue'](value);expect(emit).toHaveBeenCalledWith('update:model',model)
  }
  for(const value of [p.min-1,p.max+1,null,NaN,...(p.stepStrictly?[p.min+.5]:[])])expect(validateForm(form.content,{...form.defaults,[field.model]:value})).not.toBe('')
 }}
})
it('a missing static schema is an explicit error, never JSON fallback or automatic enable',async()=>{const v=setupComponent(PluginForm,{pluginId:'not-in-pin',modelValue:'{}'});await flush();expect(v.state.error).toContain('已有配置仍保留');v.unmount()})
it('static schemas are independent copies and example options are accessible inside disclosure',()=>{const first=staticPluginForm('hello-go')!;first.content.length=0;expect(staticPluginForm('hello-go')!.content.length).toBeGreaterThan(0);const form=staticPluginForm('hello-go')!;expect(form.content.some(n=>n.component==='VDetails')).toBe(true);expect(nodes(form.content).some(n=>n.model==='resourceNoCleanup')).toBe(true)})
it('every declared boolean, text and enum field is editable, including false/empty values, without dropping unrelated config',()=>{
 for(const manifest of inventory.manifests)for(const field of nodes(staticPluginForm(manifest.id)!.content).filter(n=>n.model&&n.component!=='VNumberInput')){
  const values=field.component==='VSwitch'?[false,true]:field.component==='VSelect'?field.props.items:['','owned value'];for(const value of values){const emit=vi.fn(),model={otherSavedKey:'retained'},render=(PluginJson as any).setup({nodes:[field],model,readonly:false},{emit}),input=render().children[0].children[1];expect(input.props.disabled).toBe(false);input.props['onUpdate:modelValue'](value);expect(emit).toHaveBeenCalledWith('update:model',{...model,[field.model]:value})}
 }
})

it('uses formal titles and complete enum controls from the same static pin',()=>{
 for(const manifest of inventory.manifests)for(const field of nodes(staticPluginForm(manifest.id)!.content).filter(n=>n.model)){expect(field.props.title).toBe(field.props.label);if(field.props.enum){expect(field.component).toBe('VSelect');expect(field.props.items).toEqual(field.props.enum)}}
 const provider=nodes(staticPluginForm('library-refresh-notify')!.content).find(n=>n.model==='provider')!;expect(provider.props.title).toBe('媒体服务器');expect(provider.props.items).toEqual(['','builtin','emby']);expect(provider.props.itemLabels.builtin).toBe('内置媒体库')
});
