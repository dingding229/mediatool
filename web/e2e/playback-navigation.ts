import {waitForLayout} from './layout-stability';
import {expect,type Page,type Locator} from '@playwright/test';
export async function accessSegment(page:Page,segment:'rules'|'summary'|'events'){
 const label={rules:'规则',summary:'访问汇总',events:'访问明细'}[segment];
 await page.getByRole('navigation',{name:'accessTab',exact:true}).getByRole('button',{name:label,exact:true}).click();
 await expect(page.locator(`[data-access-panel="${segment}"]`)).toBeVisible();
}
export async function observationMenu(page:Page,row:Locator){
 const trigger=row.getByRole('button',{name:'UA 汇总更多',exact:true});await trigger.click();
 const id=await trigger.getAttribute('aria-controls');expect(id).toBeTruthy();
 // A previous row's menu can remain visible during its exit animation; use the trigger's exact controlled popup.
 const menu=page.locator(`[id="${id}"]`);await expect(menu).toBeVisible();await waitForLayout(page);return menu;
}
export async function openObservationAction(page:Page,row:Locator,name:string){
 await accessSegment(page,'summary');
 const menu=await observationMenu(page,row);
 await menu.getByRole('menuitem',{name,exact:true}).click();
}
