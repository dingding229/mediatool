import {it,expect} from 'vitest';
import {readFileSync} from 'node:fs';
import {parse} from '@vue/compiler-sfc';
import {baseParse,type ElementNode,type RootNode} from '@vue/compiler-dom';
const read=(path:string)=>readFileSync(new URL(path,import.meta.url),'utf8');
it('I30b site status is inside the shared search form card in both modes',()=>{
 const view=read('../views/SearchWorkspaceView.vue'),ast=baseParse(parse(view).descriptor.template!.content);
 function elements(node:RootNode|ElementNode):ElementNode[]{return node.children.flatMap(n=>n.type===1?[n,...elements(n)]:[])}
 const nodes=elements(ast),card=nodes.find(n=>n.tag==='section'&&n.props.some(p=>p.type===6&&p.name==='class'&&p.value?.content==='acquire-surface'))!;
 expect(elements(card).filter(n=>n.props.some(p=>p.type===6&&p.name==='class'&&p.value?.content==='search-site-scope'))).toHaveLength(1);
 expect(view).toContain('还没有可用站点。');expect(view).toContain('to="/sites?tab=accounts"');
 expect(view).toContain('v-if="searched" class="search-helper-text"');
});
it('I30b query receipt and site helper share secondary typography without changing the status row height',()=>{
 const css=read('./acquisition.css');expect(css).toContain('.search-helper-text,.search-site-scope{color:var(--mt-text-muted);font-size:13px;line-height:1.6}');
 expect(css).toContain('.search-site-scope{display:flex;align-items:center;gap:8px;min-height:44px;margin-top:12px}');
});
