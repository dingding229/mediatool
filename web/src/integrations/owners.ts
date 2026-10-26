export function connectionOwner(kind:string):string{return({ptsite:'/sites',clouddrive2:'/connections',open115:'/connections',qbittorrent:'/connections',tmdb:'/connections',telegram:'/connections',local:'/files'}as Record<string,string>)[kind]??'/connections'}
export function connectionLabel(kind:string):string{return({ptsite:'PT 站点',clouddrive2:'云盘',open115:'云盘 · 115 授权',qbittorrent:'连接 · 下载器',tmdb:'连接 · TMDB',telegram:'通知',local:'本地文件'}as Record<string,string>)[kind]??kind}

export function connectionTab(kind:string){return ({clouddrive2:"storage",open115:"storage",tmdb:"aux",telegram:"notifications",qbittorrent:"downloader"} as Record<string,string>)[kind]??"storage"}
