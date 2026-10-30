import {it,expect} from 'vitest';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
const read=(path:string)=>readFileSync(resolve(__dirname,'..',path),'utf8');
it('scopes compact follow-list layout without replacing shared pagination or changing history',()=>{
 const view=read('views/SubscriptionWorkspaceView.vue'),css=read('subscriptions/subscriptions.css');
 expect(view).toContain("'subscription-follow-section':tab==='list'");
 expect(view).toContain('label="订阅列表分页"');expect(view).toContain('label="订阅运行历史分页"');
 expect(css).toContain('.subscription-workbench.subscription-follow-section{min-height:0;max-height:none;overflow:hidden;scrollbar-gutter:auto}');
 expect(css).toContain('.subscription-follow-section>.mt-pagination{margin:0;padding:16px 24px');
 expect(css).toContain('.subscription-follow-section .f5-subscription-results{min-height:0}');
});
it('I30 keeps the history pager in its timeline footer without changing its bindings',()=>{
 const view=read('views/SubscriptionWorkspaceView.vue'),css=read('subscriptions/subscriptions.css');
 const footer=view.slice(view.indexOf('<footer class="subscription-history-pagination">'),view.indexOf('</footer>',view.indexOf('<footer class="subscription-history-pagination">')));
 expect(footer).toContain('label="订阅运行历史分页"');expect(footer).toContain('v-model:current-page="historyPage" :total="historyTotal" v-model:page-size="historySize" @current-change="loadHistory"');
 expect(view.match(/label="订阅运行历史分页"/g)).toHaveLength(1);
 expect(view).toContain('v-if="!historyLoading&&!historyError"');
 expect(css).toContain('.subscription-history-pagination{flex:none;padding:16px 24px;border-top:1px solid var(--mt-border-color)}');
 expect(css).toContain('.subscription-history-timeline>.run-timeline,.subscription-history-pagination{padding:16px}');
});
