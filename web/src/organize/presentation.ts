import {stepState} from '../files/workbench';
export const present=(value:unknown)=>value!=null&&String(value).trim()!==''&&String(value).trim()!=='—';
export function historySteps(detail:any){
 const planned=detail?.details?.plan?.steps||[],parts=detail?.details?.items||[];
 return [...planned.map((step:any,index:number)=>({number:index+1,action:step.action==='mkdir'?'创建目录':'移动或改名',state:step.state||(detail.state==='succeeded'?'succeeded':stepState(detail.state,detail.details.completedSteps||0,index)),source:typeof step.source==='string'?step.source:step.source?.path,target:typeof step.target==='string'?step.target:step.target?.path})),...parts.map((part:any,index:number)=>({number:planned.length+index+1,action:'处理文件',state:part.state,source:part.source,target:part.destination,message:part.message}))];
}
export function scanRecords(rows:any[]){return [...new Map(rows.map(row=>[row.id,row])).values()]}
export function identityDraft(file:any){
 const name=file.name||file.path||'',match=name.match(/S(\d+)E(\d+)(?:-E?(\d+))?/i)||name.match(/(\d+)x(\d+)(?:-(\d+))?/i),seasonOnly=name.match(/(?:^|[. _-])S(\d+)(?![a-z\d])/i),episodeOnly=name.match(/(?:^|[. _-])E(\d+)(?:-E?(\d+))?/i);
 const season=file.season??(match?Number(match[1]):seasonOnly?Number(seasonOnly[1]):0),start=match?Number(match[2]):episodeOnly?Number(episodeOnly[1]):undefined,end=match?.[3]?Number(match[3]):episodeOnly?.[2]?Number(episodeOnly[2]):start;
 const explicit=Array.isArray(file.episodes)?file.episodes:(Number.isInteger(file.episode)?[file.episode]:undefined),coverage=explicit?explicit.join(','):start&&end&&end>=start&&end-start<1000?Array.from({length:end-start+1},(_,i)=>start+i).join(','):'';
 return {season,episodes:coverage};
}
