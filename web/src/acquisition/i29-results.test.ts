import {describe,it,expect} from 'vitest';
import {createSSRApp,defineComponent,h} from 'vue';
import {renderToString} from 'vue/server-renderer';
import ResourceIdentityNotice from './ResourceIdentityNotice.vue';
import {resultsEmptyTitle,resultsEmptyDescription} from './results-empty';
import type {Run} from './model';
const run:Run={id:'run',keyword:'嫁金钗',state:'succeeded',sites:[{accountId:'site',name:'站点',state:'succeeded',message:'',count:407}],resources:[],total:0,excluded:407};
async function render(excluded:number,showAll:boolean){const app=createSSRApp(ResourceIdentityNotice,{excluded,showAll});app.component('el-button',defineComponent({setup:(_,ctx)=>()=>h('button',ctx.slots.default?.())}));return renderToString(app)}
describe('I29 identity exclusion visibility',()=>{
 it('renders excluded count and explicit browse-all entry with zero results',async()=>{const html=await render(407,false);expect(html).toContain('另有 407 条因与所选媒体身份不匹配被排除');expect(html).toContain('显示全部（不按媒体身份过滤）');expect(await render(0,false)).not.toContain('显示全部')});
 it('makes browse-all mode explicit and retains delivery warning',async()=>{const html=await render(407,true);expect(html).toContain('恢复媒体身份过滤');expect(html).toContain('投递仍须通过身份校验')});
 it('distinguishes no site results, all identity excluded, other filters and running',()=>{
  expect(resultsEmptyTitle(run,false)).toBe('全部被身份过滤');expect(resultsEmptyDescription(run,false)).toContain('投递仍须通过身份校验');expect(resultsEmptyTitle(run,true)).toBe('没有匹配资源');expect(resultsEmptyTitle({...run,sites:[],excluded:0},false)).toBe('没有匹配资源');
  expect(resultsEmptyTitle({...run,excluded:0,sites:run.sites.map(s=>({...s,count:0}))},false)).toBe('站点无结果');
  expect(resultsEmptyTitle({...run,excluded:0,sites:run.sites.map(s=>({...s,state:'failed'}))},false)).toBe('没有匹配资源');
  expect(resultsEmptyTitle({...run,state:'running'},false)).toBe('站点正在查询…');
 });
});
