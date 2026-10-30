import {expect,type Page} from '@playwright/test';
export async function openHistoryMore(page:Page){if(!await page.getByRole('button',{name:'重置筛选',exact:true}).isVisible())await page.getByRole('button',{name:'更多筛选',exact:true}).click();await expect(page.getByRole('button',{name:'重置筛选',exact:true})).toBeVisible()}
export async function resetHistory(page:Page){await openHistoryMore(page);await page.getByRole('button',{name:'重置筛选',exact:true}).click();await page.getByRole('button',{name:'更多筛选',exact:true}).click();await expect(page.getByRole('button',{name:'重置筛选',exact:true})).toBeHidden()}
