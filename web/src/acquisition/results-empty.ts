import type {Run} from './model';
export function resultsEmptyTitle(run:Run,showAll:boolean){
 if(['pending','running'].includes(run.state))return '站点正在查询…';
 if(!showAll&&Number(run.excluded)>0&&Number(run.total)===0&&run.excluded===run.sites.reduce((n,s)=>n+s.count,0))return '全部被身份过滤';
 if(run.sites.length>0&&run.sites.every(s=>s.state==='succeeded')&&run.sites.every(s=>s.count===0))return '站点无结果';
 return '没有匹配资源';
}

export function resultsEmptyDescription(run:Run,showAll:boolean){
 const title=resultsEmptyTitle(run,showAll);
 if(title==='全部被身份过滤')return '站点已返回资源；可显示全部进行核对，投递仍须通过身份校验。';
 if(title==='站点无结果')return '所有站点均未返回资源，可尝试其他关键词。';
 return '若有站点失败或部分结果，请先核对上方原因。';
}
