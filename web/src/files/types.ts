export interface FileRoot{id:string;name:string;kind:'cloud'|'local';path:string;revision:number;capabilities?:FileCapabilities;connectionState?:string}
export interface FileEntry{id:string;name:string;path:string;size:number;directory:boolean;modified?:number;unsupported?:boolean;capabilities?:FileCapabilities}
export interface FileStep{action:'mkdir'|'relocate';source:FileEntry;target:string}
export interface FilePlan{id:string;connectionId:string;revision:number;backendKind:string;root:string;operation:string;items:{source:FileEntry;target:string}[];steps:FileStep[];digest:string;expiresAt:string}
export interface FileResult{id:string;state:string;completedSteps:number;message:string;plan:FilePlan}
export interface FileCapabilities {read:boolean;write:boolean;move:boolean;recycle:boolean;readOnly:boolean;role:string;reason?:string}
export interface FileBatchItem {ordinal:number;source:string;state:string;message:string;result?:FileResult}
export interface FileBatch {id:string;digest:string;state:string;expiresAt:string;createdAt:string;updatedAt:string;items:FileBatchItem[]}
