import {expect,it} from 'vitest';
import {createSSRApp,defineComponent,h} from 'vue';
import {renderToString} from '@vue/server-renderer';
import Pagination from './PagePagination.vue';
async function render(props:Record<string,unknown>){const app=createSSRApp(Pagination,props);app.component('el-select',defineComponent({setup:(_,ctx)=>()=>h('select',ctx.slots.default?.())}));app.component('el-option',defineComponent({setup:(_,ctx)=>()=>h('option',ctx.attrs)}));return renderToString(app)}
it.each([0,1,77])('shows only left-side total %s without old range',async total=>{const html=await render({total});expect(html).toContain(`共 ${total} 条`);expect(html).not.toMatch(/\d+ – \d+/);expect(html.indexOf(`共 ${total} 条`)).toBeLessThan(html.indexOf('mt-page-actions'))});
it('omits unknown count and locates loading/failure in left summary',async()=>{for(const status of ['', 'loading','error']){const html=await render({status});expect(html).not.toContain('总数未提供');expect(html).not.toContain(' 起');expect(html).not.toContain('共 undefined');if(status)expect(html.indexOf(status==='loading'?'读取中':'读取失败')).toBeLessThan(html.indexOf('mt-page-actions'))}});
