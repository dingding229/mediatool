import {it,expect} from 'vitest';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {parse} from '@vue/compiler-sfc';
import {baseParse,type ElementNode,type RootNode} from '@vue/compiler-dom';
const read=(path:string)=>readFileSync(resolve(__dirname,'..',path),'utf8');
it('both tabs retain the shared full-width padded pagination style',()=>{
 const view=read('views/SubscriptionWorkspaceView.vue'),css=read('subscriptions/subscriptions.css');
 expect(view).toContain("'subscription-follow-section':tab==='list'");
 expect(view).toContain('label="订阅列表分页"');expect(view).toContain('label="订阅运行历史分页"');
 expect(css).toContain('.subscription-workbench.subscription-follow-section{min-height:0;max-height:none;overflow:hidden;scrollbar-gutter:auto}');
 expect(css).toContain('.subscription-workbench>.mt-pagination{margin:0;padding:16px 24px;box-sizing:border-box;border-top:1px solid var(--mt-border-color)}');
 expect(css).toContain('.subscription-workbench>.mt-pagination{padding:16px}');
 expect(css).toContain('.subscription-follow-section .f5-subscription-results{min-height:0}');
});
it('I30b history and list pagers are direct card children, after the two-column results',()=>{
 const view=read('views/SubscriptionWorkspaceView.vue'),css=read('subscriptions/subscriptions.css');
 const ast=baseParse(parse(view).descriptor.template!.content);
 function elements(node:RootNode|ElementNode):ElementNode[]{return node.children.flatMap(n=>n.type===1?[n,...elements(n)]:[])}
 const card=elements(ast).find(n=>n.tag==='section'&&n.props.some(p=>p.type===6&&p.name==='class'&&p.value?.content.includes('subscription-workbench')))!;
 const pagers=card.children.filter((n):n is ElementNode=>n.type===1&&n.tag==='PagePagination');
 expect(pagers).toHaveLength(2);expect(pagers.map(n=>{const p=n.props.find(p=>p.type===6&&p.name==='label');return p?.type===6?p.value?.content:undefined})).toEqual(['订阅列表分页','订阅运行历史分页']);
 const history=view.match(/<PagePagination label="订阅运行历史分页"[^>]*\/>/g)!;expect(history).toHaveLength(1);expect(history[0]).toContain('v-model:current-page="historyPage" :total="historyTotal" v-model:page-size="historySize" @current-change="loadHistory"');
 expect(css).not.toContain('.subscription-history-timeline');expect(css).not.toContain('.subscription-history-pagination');
 expect(view).not.toContain('class="subscription-history-timeline"');
 expect(view.indexOf('label="订阅运行历史分页"')).toBeGreaterThan(view.indexOf('<RunReview :run="run"'));
});
