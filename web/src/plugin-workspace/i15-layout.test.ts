import {it,expect} from 'vitest';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {parse} from '@vue/compiler-sfc';
it('limits plugin layout to the host configuration container, retaining save gates and form bindings',()=>{
 const source=readFileSync(resolve(__dirname,'InstanceDetail.vue'),'utf8'),sfc=parse(source).descriptor;
 expect(sfc.template!.content).toContain('class="media-surface plugin-config" aria-label="插件配置"');
 expect(sfc.template!.content).toContain('<div class="plugin-config-actions">');
 expect(sfc.template!.content).toContain(':disabled="!formValid||!!op.pending||op.busy||latest||!!data.error"');
 expect(sfc.template!.content).toContain('v-model="config" :plugin-id="row.pluginId"');
 expect(sfc.styles[0]!.scoped).toBe(true);
 expect(sfc.styles[0]!.content).toContain('grid-template-columns:repeat(2,minmax(0,1fr))');
 expect(sfc.styles[0]!.content).toContain('@media(max-width:700px)');
 expect(sfc.styles[0]!.content).not.toMatch(/\.f3-plugin-surface|\.plugin-surface-mode/);
});
