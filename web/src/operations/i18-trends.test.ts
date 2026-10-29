import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {reactive} from 'vue';
import {readFileSync} from 'node:fs';
import Trends from '../views/TrendsWorkspaceView.vue';
import {setupComponent,flush,deferred} from '../test/setup-component';
const route=reactive<{query:Record<string,string>;hash:string}>({query:{hours:'24'},hash:''});
const replace=vi.fn(async(to:any)=>{route.query=to.query;route.hash=to.hash});
vi.mock('vue-router',()=>({useRoute:()=>route,useRouter:()=>({replace})}));
let fetch:ReturnType<typeof vi.fn>,view:ReturnType<typeof setupComponent>|undefined;
beforeEach(()=>{vi.useFakeTimers();route.query={hours:'24'};route.hash='';replace.mockClear();fetch=vi.fn(async()=>({ok:true,status:200,json:async()=>({data:[{at:'2026-10-29T00:00:00Z',processBytes:1048576,succeededJobs10m:3,failedJobs10m:1}]})}));vi.stubGlobal('fetch',fetch)});
afterEach(()=>{view?.unmount();view=undefined;vi.useRealTimers();vi.unstubAllGlobals()});
it('I18 moves the existing request/derived metrics and loads exactly once per range change',async()=>{
 view=setupComponent(Trends);await flush();expect(fetch).toHaveBeenCalledTimes(1);expect(fetch.mock.calls[0]![0]).toBe('/api/v1/ops/trends?hours=24');expect(view.state.trendRows[0]).toMatchObject({processMiB:1,successRate:75});expect(view.state.trendFields).toHaveLength(9);
 view.state.hours=168;await flush();expect(replace).toHaveBeenCalledWith({query:{hours:'168'},hash:''});expect(fetch).toHaveBeenCalledTimes(2);expect(fetch.mock.calls[1]![0]).toContain('hours=168');
 await vi.advanceTimersByTimeAsync(30000);expect(fetch).toHaveBeenCalledTimes(3);
 expect(fetch.mock.calls.every(([,options])=>!options?.method||options.method==='GET')).toBe(true);
});
it('I18 preserves query/hash, falls back to 24h for invalid bookmarks and fences stale responses',async()=>{
 route.query={hours:'999',keep:'yes'};route.hash='#sample';const gate=deferred<any>();fetch.mockImplementationOnce(()=>gate.promise);view=setupComponent(Trends);await flush();expect(view.state.hours).toBe(24);
 view.state.hours=1;await flush();expect(replace).toHaveBeenCalledWith({query:{hours:'1',keep:'yes'},hash:'#sample'});gate.resolve({ok:true,status:200,json:async()=>({data:[{processBytes:999999}]})});await flush();expect(view.state.trendRows[0].processMiB).toBe(1);
});
it('I18 charts stretch only when explicitly requested; ops no longer owns duplicated trend logic',()=>{
 const chart=readFileSync(new URL('./TrendChart.vue',import.meta.url),'utf8'),ops=readFileSync(new URL('./MaintenancePanel.vue',import.meta.url),'utf8');
 expect(chart).toContain('stretch:false');expect(chart).toContain('preserveAspectRatio="none"');expect(chart).toContain('preserveAspectRatio="xMidYMid meet"');expect(chart).toContain('non-scaling-stroke');
 expect(ops).not.toContain('/ops/trends');expect(ops).not.toContain('trendFields');expect(ops).not.toContain("trends:'运行趋势'");
});
