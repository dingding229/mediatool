import {expect,it} from 'vitest';
import {readFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
import postcss from 'postcss';
const read=(p:string)=>readFileSync(new URL('../'+p,import.meta.url),'utf8');
const sha=(s:string)=>createHash('sha256').update(s).digest('hex');
it('I19-6 leaves all maintenance request/paging/reconciliation functions byte-identical, apart from icon imports',()=>{
 const script=read('operations/MaintenancePanel.vue').match(/<script[^>]*>([\s\S]*?)<\/script>/)![1]!.replace("import { Check, View, Refresh, Download, Lock, Unlock, Delete, Close } from '@element-plus/icons-vue';\n",'');
 expect(sha(script)).toBe('e89b4dd415f0c5e806223d8002798f1e0360ec334c9144860131a40a66b437fa');
 const source=read('operations/MaintenancePanel.vue');expect(sha(source.slice(source.indexOf('    <ConfirmAction v-model:visible="confirm"')))).toBe('67264737fd68b758a325374b56bb1cbaf6c20d4506c185a8d7636893a44d8cc9');
});
it('I19-6 preserves every original action guard and all safety text, including recent-two backup protection',()=>{
 const source=read('operations/MaintenancePanel.vue');
 const expected=[
  ["ask('backup')",undefined,'!!op.pending||!!data.error||data.loading||!data.data?.toolsReady'],
  ["ask('drill',b)","b.state==='ready'",'!!op.pending||!!data.error||data.loading||!data.data?.toolsReady'],
  ["ask('pin',b)",'canPinBackup(b.state)','!!op.pending'],["ask('delete',b)",'canDeleteBackup(b.state)','!!op.pending||b.pinned'],
  ["ask('cleanup',d)","['failed','interrupted'].includes(d.state)",'!!op.pending'],
  ["ask('cancel',r)","['preview','approved'].includes(r.state)",'!!op.pending'],["ask('verify',r)","r.state==='completed'&&!r.verifiedAt",'!!op.pending'],
  ["showPreview('restore',restoreID)",undefined,'!restoreID||!!op.pending'],["showPreview('rollback',rollbackID)",undefined,'!rollbackID||!!op.pending'],
 ];
 for(const [click,condition,disabled]of expected){const tags=[...source.matchAll(/<el-button\b([^>]*)>/g)].map(m=>m[1]!).filter(tag=>tag.includes(`@click="${click}"`));expect(tags,click).toHaveLength(1);expect(tags[0]).toContain(`:disabled="${disabled}"`);if(condition)expect(tags[0]).toContain(`v-if="${condition}"`)}
 for(const text of ['保留最近两份成功备份，固定及维护单使用中的备份不参与自动清理；最多显示最近200条。','以下保留原命令行审批流程；网页恢复请使用上方的预览与确认。',":href=\"`/api/v1/ops/backups/${b.id}/download`\"",'label="系统备份分页"','label="恢复演练分页"','label="维护单分页"',"r.state==='approved'&&r.flow?.reason!=='approval_expired'",'register dingding229/mediatool:sha-完整SHA'])expect(source).toContain(text);
 expect(source).toContain('<el-option value="" label="请选择"/>');expect(source.match(/<el-option value="" label="请选择"\/>/g)).toHaveLength(2);
});
it.each([
 ['ui/ConfirmAction.vue','abb54b05b33a408a385aef0ad5d32b311be6d7b77ab7125c481cd10f8773278b'],
 ['ui/PreviewApprove.vue','e015b8824bee6d214b2638323540119416d7b69c82ac07a318577189b88c629a'],
])('I19-6 leaves shared confirmation/workflow unchanged: %s',(file,hash)=>expect(sha(read(file))).toBe(hash));
it.each([
 // I22 only changes policy response projection; destructive handlers remain byte-identical.
 ['maintenance.go','8b152a2b9633d5daff77fe205d5e4bb3451ea278819c73d2539d7ce2ed5c8916'],
 ['web_restore.go','e74e964aece0b1ccad84a687fac4206f83e86d0edcfc9eaa19e16c17d15bbb10'],
])('I19-6 does not alter protected endpoints or ten-minute recent-session guard: %s',(file,hash)=>{
 expect(sha(readFileSync(new URL('../../../internal/platform/httpapi/'+file,import.meta.url),'utf8'))).toBe(hash);
});
it.each([
 ['repository.go','0e400c02823bf0bc8808fda6fda69c54798385eaf6f8539e43b8f5eb72c26e21'],
 ['requests.go','df912b24bac325dfd1983341738fdfa85e4a41d76528c73480823cf393a36f84'],
])('I19-6 leaves backup-retention protection and frozen CLI approval logic unchanged: %s',(file,hash)=>{
 expect(sha(readFileSync(new URL('../../../internal/modules/maintenance/'+file,import.meta.url),'utf8'))).toBe(hash);
});
it('I19-6 style selectors affect only the named backup/CLI lists and preserve all shared modal sizing',()=>{
 const css=read('operations/presentation.css').split('/* I19-6:')[1]!.split('/* I19-7:')[0]!;postcss.parse('/* I19-6:'+css).walkRules(rule=>{for(const s of rule.selector.split(/,(?![^()]*\))/))expect(s.trim()).toMatch(/^\.t8-ops\s/)});
 expect(css).not.toMatch(/\.el-dialog|\.detail-body|\.confirm-action|\.preview-approve|\.web-restore/);
});
