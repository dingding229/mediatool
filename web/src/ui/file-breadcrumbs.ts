export function fileBreadcrumbs(rootPath:string,path:string,rootLabel='云盘',emptyPath='.'){
 const base=rootPath.split('/').filter(Boolean),relative=path===emptyPath?[]:path.split('/').filter(Boolean);
 return [{name:rootLabel,path:emptyPath as string|null},...base.map((name,i)=>({name,path:i===base.length-1?emptyPath:null})),...relative.map((name,i)=>({name:name==='.media-tool-trash'?'回收目录':name,path:relative.slice(0,i+1).join('/')}))];
}
