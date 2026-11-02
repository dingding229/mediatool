// Kept in lockstep with releasename.allowedVariable; the parity guard scans Go.
export const namingFields=['title','year','season','episode','season_episode','resolution','videoFormat','source','codec','group','ext','original'] as const;
export function insertNamingField(value:string,field:string,start=value.length,end=start){
 const token=`{${field}}`,from=Math.max(0,Math.min(start,value.length)),to=Math.max(from,Math.min(end,value.length));
 return {value:value.slice(0,from)+token+value.slice(to),cursor:from+token.length};
}
