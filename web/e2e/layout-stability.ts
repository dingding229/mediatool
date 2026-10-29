import type {Page} from '@playwright/test';
// Responsive Element controls animate geometry (e.g. 32px -> 44px buttons).
// Do not compare a cached card rect against a descendant from a later animation frame.
export async function waitForLayout(page:Page,selector='body'){
 await page.locator(selector).evaluate(async root=>{
  await document.fonts.ready;
  await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);
  await Promise.all(root.getAnimations({subtree:true})
   .filter(a=>a.effect?.getTiming().iterations!==Infinity)
   .map(a=>a.finished.catch(()=>{})));
 });
}
