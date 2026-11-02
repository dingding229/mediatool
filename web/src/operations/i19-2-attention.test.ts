// I42b: approved display/projection snapshots; destructive functions retain independent I36 hashes.
// I36: refresh display-only snapshots after removing developer details; action functions are pinned in i36-business-functions.test.ts.
import {expect,it} from 'vitest';
import {readFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
import postcss from 'postcss';
const read=(p:string)=>readFileSync(new URL('../'+p,import.meta.url),'utf8');
const sha=(s:string)=>createHash('sha256').update(s).digest('hex');
it('I19-2 leaves system detection and dashboard loading unchanged (I26a expands connection buttons)',()=>{
 const runtime=read('operations/RuntimeChecks.vue').match(/<script[^>]*>([\s\S]*?)<\/script>/)![1]!.replace("import { View, Setting } from '@element-plus/icons-vue';\n",'');
 expect(sha(runtime)).toBe('157623eefe87f885763540486dec6fd2f9b030bf635f8c65d7aaec1a22bba467');
 // I26a explicitly permits adding CD2/qB to the same receipt/reconciliation flow.
 // Keep freezing the loader and shared header; behavior is covered in readiness-test and i26a-cards.
 expect(sha(read('dashboard/useDashboard.ts'))).toBe('09017bde004391cd4b81c71843309caf24b2e8ceaa4652578cb4fcdea0d108fc');
 expect(sha(read('ui/PageHeader.vue'))).toBe('e03eb2e098b252e7e494d15c1601c80f3915aaf8ccd60bfd5921f85938057de4');
});
it('I19-2 retains full detail data and original confirmation without any queue controls',()=>{
 const source=read('operations/RuntimeChecks.vue');
 for(const token of ['mt-list-row ops-runtime-row ops-worker-row','mt-list-row ops-runtime-row ops-queue-row','mt-list-row ops-runtime-row ops-schedule-row','detail=worker;open=true',"s.link?.startsWith('/')",':to="s.link"','不重放失败业务或修改媒体文件。','confirm-text="确认运行检查"',':disabled="!!op.pending||op.busy"'])expect(source).toContain(token);
 const queues=source.split('aria-label="队列列表"')[1]!.split('</section>')[0]!;expect(queues).not.toMatch(/@click|el-button|RouterLink/);
});
it('I19-2 confines all new CSS to operations and attention, never global homepage or modal sizing',()=>{
 const source=read('operations/presentation.css').split('/* I19-2:')[1]!.split('/* I19-3:')[0]!;
 postcss.parse('/* I19-2:'+source).walkRules(rule=>{for(const selector of rule.selector.split(/,(?![^()]*\))/))expect(selector.trim()).toMatch(/^\.t8-ops\s/)});
 expect(source).not.toMatch(/\.is-overview|\.detail-body|\.el-dialog|\.confirm-action/);
 expect(source).toContain('.ops-attention-dashboard .service-readiness');expect(source).toContain('.ops-attention-dashboard .dashboard-inbox');
});

it('keeps worker business details without exposing the operations object',()=>{const source=read('operations/RuntimeChecks.vue');expect(source).not.toContain('detail=health.data');expect(source).toContain('queueLabel(detail.queue)');expect(source).toContain('date(detail.lastSeenAt)')});
