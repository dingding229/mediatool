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
