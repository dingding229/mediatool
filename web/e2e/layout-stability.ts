import type {Page} from '@playwright/test';
// Responsive Element controls animate geometry (e.g. 32px -> 44px buttons).
// Do not compare a cached card rect against a descendant from a later animation frame.
export async function waitForLayout(page:Page,selector='body'){
 await page.locator(selector).evaluate(async root=>{
  await document.fonts.ready;
  await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);
  // Element Plus animates .el-overlay-dialog, an ancestor of the dialog.
  // Descendant-only waits can sample its final translateY frames as layout drift.
  const animations=new Set(root.getAnimations({subtree:true}));
  for(let parent=root.parentElement;parent;parent=parent.parentElement)
   for(const animation of parent.getAnimations())animations.add(animation);
  await Promise.all([...animations]
   .filter(a=>a.effect?.getTiming().iterations!==Infinity)
   .map(a=>a.finished.catch(()=>{})));
 });
}
