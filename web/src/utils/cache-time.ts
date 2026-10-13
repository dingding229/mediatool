export function cacheTime(kind:'tmdb'|'search',row:{ttlSeconds:number;state:string;reusable?:boolean},elapsed=0):string{
 if(kind==='search' && ['pending','running'].includes(row.state))return '查询中';
 if(kind==='search'&&!row.reusable)return ['failed','invalid','snapshot_unavailable'].includes(row.state)?'不可复用':'历史结果';
 if(row.ttlSeconds<0)return '无自动到期';
 const seconds=Math.max(0,row.ttlSeconds-Math.max(0,Math.floor(elapsed)));
 if(!seconds)return kind==='tmdb'?'已到期':'历史结果';
 if(kind==='tmdb')return seconds<36?'不足 0.01 小时':`${(seconds/3600).toFixed(2)} 小时`;
 const hours=Math.floor(seconds/3600),minutes=Math.floor(seconds%3600/60),rest=seconds%60;
 return hours?`${hours} 小时 ${minutes} 分` : minutes?`${minutes} 分 ${rest} 秒`:`${rest} 秒`;
}
