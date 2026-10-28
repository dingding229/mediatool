import {it,expect} from 'vitest';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
it('download rows have no persistent stage height or inherited progress width cap',()=>{
 const css=readFileSync(resolve(__dirname,'download-presentation.css'),'utf8');
 expect(css).toContain('.download-content-stage{min-width:0}');
 expect(css).not.toMatch(/\.download-content-stage\{[^}]*min-height/);
 expect(css).toContain('.t8-download-row .download-progress>.el-progress{width:100%;max-width:none;margin:0}');
 expect(css).toContain('.download-list-footer.is-loading{visibility:hidden}');
 const view=readFileSync(resolve(__dirname,'../views/DownloadsWorkspaceView.vue'),'utf8');
 expect(view).toContain(':inert="loading.tasks" :aria-hidden="loading.tasks"');
 expect(view).toContain('loading.tasks&&!tasks.length');
});
