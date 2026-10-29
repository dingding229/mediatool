import {expect,it} from 'vitest';
import {readFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
import postcss from 'postcss';
const read=(p:string)=>readFileSync(new URL('../'+p,import.meta.url),'utf8');
const sha=(s:string)=>createHash('sha256').update(s).digest('hex');
it('I19-7 changes no restore/execute/reconcile/session/polling logic, only an icon import',()=>{
 const script=read('operations/WebRestorePanel.vue').match(/<script[^>]*>([\s\S]*?)<\/script>/)![1]!.replace("import { View, Refresh } from '@element-plus/icons-vue';\n",'');expect(sha(script)).toBe('78570a1b5a794a6627b92511a27a6c6fecaf3248d8c7d29c50534d40a9d1ed7d');
});
it('I19-7 keeps the complete preview/confirmation markup and its warnings, exact phrase and digest display untouched',()=>{
 const source=read('operations/WebRestorePanel.vue');expect(sha(source.slice(source.indexOf(' <DetailDrawer')))).toBe('048443cf74b45d5f6c111add1e32cf2920ee2b7dc069ab8e8068daf01c588b9f');
});
it('I19-7 every disabled/click/confirm binding is identical and in original order',()=>{
 const source=read('operations/WebRestorePanel.vue'),bindings=[...source.matchAll(/(:disabled|@click|@confirm|:confirm-phrase)="([^"]*)"/g)].map(m=>[m[1],m[2]]);
 expect(bindings).toEqual([
  [':disabled','busy||previewLoading||!!props.backupError||props.backupsLoading'],['@click','inspect(b.id)'],
  [':disabled','busy'],['@click','reconcile(r.id)'],
  [':disabled','busy||previewLoading||!!props.backupError||props.backupsLoading'],['@click','inspect(r.snapshotId)'],
  [':disabled','busy||!!writeError'],['@click','confirm=true'],[':confirm-phrase','preview?.confirmText'],['@confirm','execute'],
 ]);
});
it('I19-7 keeps the original selection filter/array, snapshot action conditions and every visible risk sentence',()=>{
 const source=read('operations/WebRestorePanel.vue');for(const token of ["(props.backupError?[]:props.backups).filter(b=>b.state==='ready')",'v-for="r in records"',"r.state==='unknown'","r.snapshotId&&['completed','rolled_back'].includes(r.state)",'v-if="!error"','先预览校验，再输入备份确认短语。确认需10分钟内的新近登录；执行前自动保留恢复前快照。','账号、115授权、连接、设置、媒体记录、插件配置及KV全部随备份回退；不覆盖现存播放文件。数据库恢复后旧任务暂停，需逐项核对。','包含数据库、清单和登记的受管输出','上次预览校验通过；再次执行前会重检','恢复前快照 {{r.snapshotId}}'])expect(source).toContain(token);
 expect(source).not.toMatch(/<select|slice\(|PagePagination/); // No new selection defaults or render slices hide pending receipts.
});
it('I19-7 presentation stays in the web-restore list scope, not shared dialogs or CLI',()=>{
 const css=read('operations/presentation.css').split('/* I19-7:')[1]!;postcss.parse('/* I19-7:'+css).walkRules(rule=>{for(const selector of rule.selector.split(/,(?![^()]*\))/))expect(selector.trim()).toMatch(/^\.t8-ops\s\.ops-web-/)});expect(css).not.toMatch(/\.el-dialog|\.detail-body|\.confirm-action|\.ops-cli|\.ops-backup-row/);
});
it.each([
 ['operations/MaintenancePanel.vue','1e15eaa8527f4253484641726c136ddc4a684a83d02cb74eaf58612027313aa6'],
 ['workflows/restore-session.ts','0215778c804dc3c98505816adcc505a2dc0cbb2cf176f29803e843a9c203e425'],
 ['ui/ConfirmAction.vue','abb54b05b33a408a385aef0ad5d32b311be6d7b77ab7125c481cd10f8773278b'],
 ['ui/DetailDrawer.vue','89d028ea7bb8716a17edf4af45a9095b5bae5378d3bdf34dfee310d2224a2d76'],
])('I19-7 preserves parent selector values, restore session and shared dialogs unchanged: %s',(file,hash)=>expect(sha(read(file))).toBe(hash));
it('I19-7 keeps the existing rendered risk text and review/rollback receipt labels without adding local expiry/default behavior',()=>{
 const source=read('operations/WebRestorePanel.vue');for(const token of ['结果未知，需核对','失败后已自动回滚','未执行恢复','没有再次执行恢复','已排队，尚未恢复。请查看执行记录；成功后需要重新登录。','先核对执行记录；需要重新登录时，登录后重新预览。'])expect(source).toContain(token);
 expect(source).toContain('SHA256 {{b.digest}}');expect(source).toContain("b.origin==='pre-restore'?'预览回滚到恢复前':'预览网页恢复'");
});
it('I19-7 does not change backend expiry/digest/admission or failure-compensation implementation',()=>{
 const source=readFileSync(new URL('../../../internal/modules/maintenance/web_restore.go',import.meta.url),'utf8');expect(sha(source)).toBe('5a7bed81b26e183f3e34e1bd148faf2f9e571519f5328fa4b122727218df46d0');
});
