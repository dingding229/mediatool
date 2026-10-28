import {expect,type Page} from '@playwright/test';
const labels:Record<string,string>={off:'关闭（全部允许）',whitelist:'白名单（仅允许匹配规则）',blacklist:'黑名单（拒绝匹配规则）',rules:'白名单规则',blacklistRules:'黑名单规则',whitelisted:'已在白名单',unlisted:'未在白名单',blacklisted:'已在黑名单','':'全部访问'};
export async function chooseUASelect(page:Page,name:string,value:string){
 const input=page.getByRole('combobox',{name,exact:true});await page.locator('.el-select').filter({has:input}).click();const id=await input.getAttribute('aria-controls');expect(id).toBeTruthy();await page.locator(`[id="${id}"]`).getByRole('option',{name:labels[value],exact:true}).click();
}
export async function expectUASelect(page:Page,name:string,value:string){await expect(page.locator('.el-select').filter({has:page.getByRole('combobox',{name,exact:true})})).toContainText(labels[value]!)}
