const known:Record<string,string>={'zh-cn':'简体中文','zh-hans':'简体中文','zh-sg':'简体中文','zh-tw':'繁体中文','zh-hant':'繁体中文','zh-hk':'繁体中文',zh:'中文',en:'英语',eng:'英语',ja:'日语',jpn:'日语',ko:'韩语',kor:'韩语',chi:'中文'};
const names=new Intl.DisplayNames(['zh-CN'],{type:'language',fallback:'code'});
export function languageName(code:string){if(known[code.toLowerCase()])return known[code.toLowerCase()]!;try{return names.of(code.replaceAll('_','-'))||code}catch{return code}}
