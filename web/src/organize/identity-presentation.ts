export interface MigrationPath {path:string;kind?:string;operation:string}
export function migrationPathGroups(operations:MigrationPath[]){
 const groups=new Map<string,{kind:string;source:MigrationPath[];target:MigrationPath[]}>();
 for(const operation of operations){
  const kind=operation.kind||'输出文件';
  if(!groups.has(kind))groups.set(kind,{kind,source:[],target:[]});
  const group=groups.get(kind)!;
  (operation.operation==='archive'?group.source:group.target).push(operation);
 }
 return [...groups.values()];
}
