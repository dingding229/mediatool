// I42c: approved display/import changes; original mutation functions remain independently pinned.
// I42b: approved display/projection snapshots; destructive functions retain independent I36 hashes.
import {stripMetadataImport} from '../test/metadata-import';
// I38: approved global pagination/defaults, FilePicker adapters, display-only imports/layout and version projection. Payload/action guards remain.
// I36: refresh display-only snapshots after removing developer details; action functions are pinned in i36-business-functions.test.ts.
import {expect,it} from 'vitest';
import {readFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
import postcss from 'postcss';
import {cacheTime} from '../utils/cache-time';
const read=(p:string)=>stripMetadataImport(readFileSync(new URL('../'+p,import.meta.url),'utf8'));
const sha=(s:string)=>createHash('sha256').update(s).digest('hex');
it.each([
 ['CachePanel.vue',"import { View, Delete, Search, Picture } from '@element-plus/icons-vue';\n",'f8ab03cc615948eec8a096d567cf4187a8c8b4155358fcb823b2bb819806e078'],
 ['MaintenancePanel.vue',"import { Check } from '@element-plus/icons-vue';\n",'f356d30bc5f9c85dd62ce0bb00bfcdde0d6fea091bd02113dde46b5ec74fc5b6'],
])('I19-3 preserves all request/state/confirmation/paging code in %s',(file,icons,hash)=>{
 const script=read('operations/'+file).match(/<script[^>]*>([\s\S]*?)<\/script>/)![1]!.replace("import { Check, View, Refresh, Download, Lock, Unlock, Delete, Close } from '@element-plus/icons-vue';\n","import { Check } from '@element-plus/icons-vue';\n").replace(icons,'');expect(sha(script)).toBe(hash);
});
it('I19-3 keeps its alert section and all shared dangerous confirmation markup unchanged after approved D/F layout work',()=>{
 const template=read('operations/MaintenancePanel.vue');
 const a=template.indexOf("    <section v-else-if=\"section==='alerts'\""),b=template.indexOf('    <PolicyPanel v-else/>',a);
 expect(sha(template.slice(a,b))).toBe('35940f54e1af106c6ef265ca45acd94ca1d60e01fc249e50b5f2355acecc5a66');
 expect(sha(template.slice(template.indexOf('    <ConfirmAction v-model:visible="confirm"')))).toBe('b4f2072e1764e09a891dce2dddef7dcb831bf9768f2eb5388838809d2f81776e');
});
it('I19-3 keeps exact time semantics, including no expiry, elapsed TTL and historical results',()=>{
 expect(sha(read('utils/cache-time.ts'))).toBe('022931a45cdb44ce23386689bc29d7d6bb7d7c15b7ea95828cce716dcd4a7e79');
 expect(cacheTime('tmdb',{ttlSeconds:3600,state:'cached'},1800)).toBe('剩余约 30 分钟');expect(cacheTime('tmdb',{ttlSeconds:-1,state:'cached'})).toBe('无自动到期');
 expect(cacheTime('search',{ttlSeconds:3600,state:'succeeded',reusable:false})).toBe('历史结果');expect(cacheTime('search',{ttlSeconds:60,state:'running',reusable:true})).toBe('查询中');
});
it('I19-3 reuses lists and pagination while keeping acknowledgement separate from recovery',()=>{
 const cache=read('operations/CachePanel.vue'),alerts=read('operations/MaintenancePanel.vue');
 for(const token of ['usePageSize(25)','mt-list-row ops-cache-row','mt-list-columns ops-cache-columns',':to="\'/search/results/\'+encodeURIComponent(row.id)"','/settings?tab=automation',':disabled="!!op.pending"'])expect(cache).toContain(token);
 expect(cache.indexOf('<p class="ops-cache-note ops-cache-ttl">')).toBeLessThan(cache.indexOf('</section>'));
 for(const token of ['mt-list-row ops-alert-row',"a.state==='active'&&!a.acknowledged_at",':disabled="!!op.pending"','a.display.badgeLabel','v-else-if="a.acknowledged_at">已确认','确认告警不等于问题已解决','/connections?tab=notifications','label="告警记录分页"'])expect(alerts).toContain(token);
 expect(alerts).not.toContain('已恢复');
});
it('I19-3 CSS stays confined to operations and never changes global dialogs or time controls',()=>{
 const source=read('operations/presentation.css').split('/* I19-3:')[1]!.split('/* I19-4:')[0]!;postcss.parse('/* I19-3:'+source).walkRules(rule=>{
  for(const selector of rule.selector.split(/,(?![^()]*\))/))expect(selector.trim()).toMatch(/^\.t8-ops\s/);
 });expect(source).not.toMatch(/\.el-dialog|\.detail-body|\.is-overview|\.confirm-action/);
});
