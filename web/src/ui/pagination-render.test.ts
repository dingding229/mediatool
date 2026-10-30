import {expect,it} from 'vitest';
import {createSSRApp,defineComponent,h} from 'vue';
import {renderToString} from '@vue/server-renderer';
import Pagination from './PagePagination.vue';
async function render(props:Record<string,unknown>){const app=createSSRApp(Pagination,props);app.component('el-select',defineComponent({setup:(_,ctx)=>()=>h('select',ctx.slots.default?.())}));app.component('el-option',defineComponent({setup:(_,ctx)=>()=>h('option',ctx.attrs)}));return renderToString(app)}
it('renders no navigation or summary for zero records',async()=>{expect(await render({total:0})).toBe('<!---->')});
it.each([1,10,25])('single page %s has only its true count',async total=>{const html=await render({total});expect(html).toContain(`共 ${total} 条`);expect(html).not.toContain('<button');expect(html).not.toContain('<select');expect(html).not.toContain('每页')});
it('multi-page lists keep all four navigation buttons and standard sizes',async()=>{const html=await render({total:77});for(const label of ['首页','上一页','下一页','末页'])expect(html).toContain(label);for(const n of [10,25,50,100])expect(html).toContain(`value="${n}"`);expect(html).not.toContain('200');expect(html.indexOf('共 77 条')).toBeLessThan(html.indexOf('mt-page-actions'))});
it('infers first terminal cursor page only; never fabricates later totals',async()=>{expect(await render({itemCount:0,hasNext:false})).toBe('<!---->');const one=await render({itemCount:7,hasNext:false});expect(one).toContain('共 7 条');expect(one).not.toContain('<button');const later=await render({itemCount:7,hasNext:false,currentPage:2});expect(later).not.toContain('共 7 条');expect(later).toContain('上一页')});
it('omits unknown count and keeps loading/failure visible',async()=>{for(const status of ['loading','error']){const html=await render({status});expect(html).not.toContain('共 undefined');expect(html).toContain(status==='loading'?'读取中':'读取失败')}});

it('single-page counts opt into the compact global spacing without changing multipage touch targets',async()=>{expect(await render({total:1})).toContain('is-single-page');expect(await render({total:100})).not.toContain('is-single-page')});
