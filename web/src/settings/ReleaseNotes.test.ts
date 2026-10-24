import {afterAll,beforeAll,describe,expect,it} from 'vitest';
import {chromium,type Browser} from '@playwright/test';
import {createServer,type ViteDevServer} from 'vite';
import vue from '@vitejs/plugin-vue';
import {resolve,join} from 'node:path';
import {mkdtempSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
const cacheDir=mkdtempSync(join(tmpdir(),'media-tool-h1-notes-'));
let browser:Browser,server:ViteDevServer,origin:string;
beforeAll(async()=>{
 server=await createServer({configFile:false,root:resolve(import.meta.dirname,'../..'),cacheDir,optimizeDeps:{noDiscovery:true,include:['vue','marked','dompurify']},plugins:[vue(),{name:'release-notes-test-entry',resolveId(id){if(id==='/h1-entry.js')return '\0h1-entry'},load(id){if(id==='\0h1-entry')return `import {createApp,h} from 'vue';import Notes from '/src/settings/ReleaseNotes.vue';createApp({render:()=>h(Notes,window.h1Props)}).mount('#app')`}}],server:{host:'127.0.0.1',port:0}});await server.listen();origin=server.resolvedUrls!.local[0]!;
 browser=await chromium.launch();
},30000);
afterAll(async()=>{await browser?.close();await server?.close();rmSync(cacheDir,{recursive:true,force:true})});
async function render(notes:string,expanded=true,requests:string[]=[]){
 const page=await browser.newPage();page.on('request',r=>requests.push(r.url()));await page.addInitScript(props=>{(window as unknown as {h1Props:unknown}).h1Props=props},{notes,tag:'v0.29.0',expanded});
 await page.route('**/h1-harness',r=>r.fulfill({contentType:'text/html',body:'<div id="app"></div><script type="module" src="/h1-entry.js"></script>'}));
 await page.goto(origin+'h1-harness');await page.locator('summary').waitFor();return page;
}
describe('release-notes real-browser sanitization',()=>{
 it('renders headings, lists, bold, inline code and safe links, and collapses long notes',async()=>{
  const page=await render('# 标题\n\n- **新增** `字幕`\n\n[详情](https://example.test/release)');
  try{expect(await page.locator('h1').textContent()).toBe('标题');expect(await page.locator('li strong').textContent()).toBe('新增');expect(await page.locator('li code').textContent()).toBe('字幕');expect(await page.locator('a').getAttribute('href')).toBe('https://example.test/release');expect(await page.locator('a').getAttribute('target')).toBe('_blank');expect(await page.locator('a').getAttribute('rel')).toBe('noopener noreferrer');await page.locator('summary').click();expect(await page.locator('.release-notes-body').isVisible()).toBe(false)}finally{await page.close()}
 });
 it('drops raw HTML, scripts, images, event handlers and dangerous link protocols before display',async()=>{
  const requests:string[]=[];
  const view=await render('<script>window.h1Injected=true</script>\n\n<img src="https://evil.invalid/a" onerror="window.h1Injected=true">\n\n<iframe src="https://evil.invalid/b"></iframe>\n\n![图片](https://evil.invalid/c)\n\n[恶意](javascript:alert%281%29) [数据](data:text/html,evil) [畸形](http://%5B)\n\n<b onclick="alert(1)">不透传粗体</b>',true,requests);
  try{expect(await view.locator('script:not([type="module"]),img,iframe,svg,[onclick],[onerror]').count()).toBe(0);expect(await view.locator('b').count()).toBe(0);expect(await view.locator('.release-notes a[href]').count()).toBe(0);expect(await view.locator('.release-notes a[target="_blank"][rel="noopener noreferrer"]').count()).toBe(3);expect(await view.evaluate(()=>Boolean((window as unknown as {h1Injected?:boolean}).h1Injected))).toBe(false);expect(requests.filter(url=>url.includes('evil.invalid'))).toEqual([])}finally{await view.close()}
 });
 it('keeps older notes collapsed when requested',async()=>{const page=await render('旧版本说明',false);try{expect(await page.locator('details').getAttribute('open')).toBe(null);expect(await page.locator('.release-notes-body').isVisible()).toBe(false)}finally{await page.close()}});
});
