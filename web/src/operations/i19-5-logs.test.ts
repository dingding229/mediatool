// I36: refresh display-only snapshots after removing developer details; action functions are pinned in i36-business-functions.test.ts.
import {expect,it} from 'vitest';
import {readFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
import postcss from 'postcss';
const read=(p:string)=>readFileSync(new URL('../'+p,import.meta.url),'utf8');
const sha=(s:string)=>createHash('sha256').update(s).digest('hex');
it('I19-5 confines every new selector to the operations log console, never global dialogs or log settings',()=>{
 const css=read('operations/presentation.css').split('/* I19-5:')[1]!.split('/* I19-6:')[0]!;
 postcss.parse('/* I19-5:'+css).walkRules(rule=>{for(const selector of rule.selector.split(/,(?![^()]*\))/))expect(selector.trim()).toMatch(/^\.t8-ops\s\.f3-log-console(?:\s|>)/)});
 expect(css).not.toMatch(/\.el-dialog|\.ops-log-settings|\.detail-body|\.ops-policy/);
});
it('I19-5 changes only console layout, keeps raw text wrapping and semantic level colors, and reserves only first load',()=>{
 const css=read('operations/presentation.css').split('/* I19-5:')[1]!.split('/* I19-6:')[0]!;
 for(const token of ['block-size:auto','max-block-size:clamp(25rem,62vh,42rem)',':has(>.state-loading)','var(--mt-bg-surface)','var(--mt-text-primary)','[data-level=info]','var(--mt-color-primary)','var(--mt-color-warning)','var(--mt-color-danger)','grid-column:1/-1;grid-row:2;width:100%','min-height:44px'])expect(css).toContain(token);
 expect(css).not.toMatch(/text-overflow|line-clamp|\.u6-raw\{[^}]*display:none|#[0-9a-f]{3,8}/i);
});
it.each([
 ['operations/LogsPanel.vue','6aef73460c2f75ed5e655dec109d4e8f5144275e7bd5cb643ea9ec70c79b03f8'],
 ['logs/format.ts','55b8c1f115e293fdd87c02807cdaf1b2ea805c43b1192bb671d92b710e7a56e8'],
 ['ui/stability.css','9d8f48ec8b03126f4a0f655f02182da6d6a7d19bbf860fab495c1c37132d27d6'],
 ['api/client.ts','bca6d6605ca0a9507b6f9975a05f388031da8b705df6896bddf81432052b42e4'],
])('I19-5 preserves console/transport; normalize only I26b approved settings and field tokens: %s',(file,hash)=>{
 let source=read(file);
 if(file==='operations/LogsPanel.vue')source=source.replace('<label class="u5-field ops-log-friendly">详情中显示友好说明<ElSwitch v-model="friendly" aria-label="友好说明"/></label>','<label class="u5-check"><input v-model="friendly" type="checkbox" aria-label="友好说明"/>详情中显示友好说明</label>');
 if(file==='ui/stability.css')for(const [selector,radius,padding] of [['.f3-log-filters>label>input','7px','8px'],['.f3-log-footer select','6px','4px']]){
  const start=source.indexOf(selector+'{'),end=source.indexOf('}',start),rule=source.slice(start,end);
  source=source.slice(0,start)+rule.replace('border-radius:var(--el-border-radius-base)','border-radius:'+radius).replace('padding:var(--mt-field-padding)','padding:'+padding)+source.slice(end);
 }
 expect(sha(source)).toBe(hash);
});
