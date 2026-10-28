import {expect,it} from 'vitest';
import {readFileSync} from 'node:fs';
const read=(file:string)=>readFileSync(new URL(file,import.meta.url),'utf8');
it('uses the I9 shared row/action classes rather than independent card styles',()=>{
 const view=read('../views/OrganizeWorkspaceView.vue'),scan=read('./ScanPanel.vue'),access=read('../playback/AccessPanel.vue');
 for(const source of [view,scan,access]){expect(source).toContain('mt-list-row');expect(source).toContain('mt-list-actions');expect(source).toContain('mt-list-action-icon')}
 expect(view).not.toMatch(/assetView|asset-collection|asset-card|海报墙|列表视图/);expect(scan).toContain('control.canCancel');expect(scan).toContain('control.canRetry');
 const shared=read('../ui/consistency.css');expect(shared).toContain('.mt-list-row.mt-list-row{box-sizing:border-box;min-height:56px;padding:10px 0');expect(shared).toContain('width:44px;height:44px;min-width:44px;min-height:44px');
});
it('retains selection limits and page query defaults and uses only supplied timestamps',()=>{
 const view=read('../views/OrganizeWorkspaceView.vue'),sizes=read('./useOrganize.ts');expect(view).toContain('state.selected.candidates.length>=20');expect(view).toContain('state.selected.assets.length>=20');expect(view).toContain("state.toggle('candidates'");expect(view).toContain("state.toggle('assets'");expect(view).toContain('待核对作品');expect(view).toContain('date(candidate.updatedAt)');
 expect(sizes).toContain("candidates:usePageSize(50,'candidatesPage'),assets:usePageSize(50,'assetsPage'),scans:usePageSize(8,'scansPage')");
 const assets=view.slice(view.indexOf('aria-label="已入库资产列表"'),view.indexOf("v-show=\"tab==='history'\""));expect(assets).not.toMatch(/<time|date\(|接口未提供/);expect(read('./ScanPanel.vue')).toContain('date(scan.updatedAt||scan.createdAt)');
});
it('keeps I9 page-specific columns and mobile layout after extracting common declarations',()=>{
 const access=read('../playback/AccessPanel.vue');expect(access).toContain('.ua-rule-columns,.ua-rule-row{grid-template-columns:');expect(access).toContain('grid-template-areas:"identity identity state" "value actions actions"');expect(access).not.toContain('.ua-row-actions :deep(.el-button)');
});
