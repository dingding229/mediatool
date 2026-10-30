import {expect,type Page} from '@playwright/test';
export async function openMediaEditor(page:Page,tab:'信息'|'图片'='信息'){
 await page.getByRole('dialog',{name:'作品详情',exact:true}).locator('.catalog-detail-actions').getByRole('button',{name:'编辑',exact:true}).click();
 const editor=page.getByRole('dialog',{name:'编辑作品',exact:true});await expect(editor).toBeVisible();await editor.getByRole('button',{name:tab,exact:true}).click();await expect(editor.locator(tab==='信息'?'#metadata-title':'.image-row').first()).toBeVisible();return editor;
}
