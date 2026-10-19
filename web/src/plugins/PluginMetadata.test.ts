import {expect,it} from 'vitest'
import PluginMetadata from './PluginMetadata.vue'
import {setupComponent} from '../test/setup-component'
import review from '../../../internal/platform/httpapi/testdata/contracts/plugin-library-review.json'
it('uses only local pinned image resources and plain version history',()=>{
 const valid=setupComponent(PluginMetadata,{metadata:review.current})
 expect(valid.state.icon).toBe('/plugins/hello-go/icon.svg');expect(valid.state.history).toContainEqual(['2.0.0','新增设置权限并收紧资源范围']);valid.unmount()
 for(const icon of ['https://external.invalid/icon.svg','//external.invalid/icon','../../escape','data:image/svg+xml,<svg/>']){const x=setupComponent(PluginMetadata,{metadata:{...review.current,icon}});expect(x.state.icon).toBe('');x.unmount()}
})
