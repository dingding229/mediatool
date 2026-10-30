// I36: refresh display-only snapshots after removing developer details; action functions are pinned in i36-business-functions.test.ts.
import {expect,it} from 'vitest';
import {readFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
import postcss from 'postcss';
const read=(p:string)=>readFileSync(new URL('../'+p,import.meta.url),'utf8');
const sha=(s:string)=>createHash('sha256').update(s).digest('hex');
it.each([
 ['operations/PolicyPanel.vue','a38f122b7e1c996ba52cb4b5a98429ca77c545a9a0985a1be8015df4b2aff445'],
 ['operations/LogsPanel.vue','d2578d872a94b921423b13270444bd8909e0fca91e6b48caf6810976035b928e'],
])('I19-4 leaves requests, settings revisions and Debug logic byte-identical in %s',(file,hash)=>expect(sha(read(file).match(/<script[^>]*>([\s\S]*?)<\/script>/)![1]!)).toBe(hash));
it.each([
 ['ui/CronInput.vue','8270404e17a385b5f73d72f8c87be1d415967aab5c6b3fd86b3cbe2195304cfa'],
 ['ui/cron-validation.ts','8a7e25b70ddf9cea1661e803d5844daec0f7f944b81b074a72b671c21d72e946'],
 ['settings/revision.ts','01c4f44fdb5a559bbb839d636d02f58af71481a1c10b21aa8325978d3cdb12a9'],
 ['ui/DetailDrawer.vue','eaa99d7f3387e32631c19655b2dac03f45e0c19bcaa8fb40d31fa7ec1b3c29a2'],
])('I19-4 does not change shared validation, concurrency or dialog behavior: %s',(file,hash)=>expect(sha(read(file))).toBe(hash));
it('I19-4 does not touch the log console, filters, exports or cleanup confirmations outside settings',()=>{
 const source=read('operations/LogsPanel.vue'),a=source.indexOf('    <DetailDrawer v-model:visible="settings"'),b=source.indexOf('    <el-dialog v-model="copyOpen"',a);
 expect(sha(source.slice(0,a)+'SETTINGS\n'+source.slice(b))).toBe('dd62a3e009b48b5f3f06bda2709c1281af061e5073c547cf742a26f1fc54446d');
});
it('I19-4 retains native numeric validation and all original operation guards',()=>{
 const policy=read('operations/PolicyPanel.vue'),logs=read('operations/LogsPanel.vue');
 for(const token of ['purpose="backup"',':disabled="op.busy||!!op.pending"','@valid="cronValid=$event"',':min="field.min"',':max="field.max"',':step="field.step||1" required','!!op.pending||!!latest||!!read.error||!cronValid','@click="reloadPolicy"'])expect(policy).toContain(token);
 for(const token of [':min="1" :max="730"',':min="1" :max="90"','@change="durationTouched=true"',':disabled="policy.conflict.value"','mt-list-row ops-log-file-row','bytes(file.size)','formatLogTime(file.modified)','不采集登录请求体。'])expect(logs).toContain(token);
 const files=logs.split('aria-label="日志文件列表"')[1]!.split('</section>')[0]!;expect(files).not.toMatch(/@click|el-button|<a |RouterLink/);
});
it('I19-4 styles are policy-scoped or use the unique teleported log-settings class, not global modal rules',()=>{
 const css=read('operations/presentation.css').split('/* I19-4:')[1]!.split('/* I19-5:')[0]!;
 postcss.parse('/* I19-4:'+css).walkRules(rule=>{for(const s of rule.selector.split(/,(?![^()]*\))/))expect(s.trim()).toMatch(/^\.t8-ops\s\.ops-policy|^\.el-dialog\.ops-log-settings\s/)});
 expect(css).not.toMatch(/\.detail-body\s*\{|max-height|\.el-dialog__body|\.el-dialog__footer/);
});
