// Client feedback only. The backend additionally rejects duplicate raw JSON
// keys and repeats all validation before encrypting or using credentials.
export function accountHeadersError(raw:string):string {
 if(raw==='')return ''
 if(new TextEncoder().encode(raw).length>16384)return '附加鉴权头过长，最多16384字节。'
 let value:unknown
 try{value=JSON.parse(raw)}catch{return '附加鉴权头需填写完整JSON对象，例如 {"Authorization":"Bearer …"}。'}
 if(value===null||Array.isArray(value)||typeof value!=='object')return '附加鉴权头必须是JSON对象；填写 {} 可清除，留空保留。'
 const entries=Object.entries(value)
 // This flat object is already valid JSON. Match string tokens followed by
 // a colon to retain raw key occurrences that JSON.parse would overwrite.
 const rawNames=new Set<string>()
 for(const match of raw.matchAll(/("(?:[^"\\]|\\.)*")\s*:/g)){
  const name=(JSON.parse(match[1]) as string).toLowerCase()
  if(rawNames.has(name))return '鉴权头名称不能重复，大小写不同也视为同一个头。'
  rawNames.add(name)
 }
 if(entries.length>16)return '附加鉴权头最多16项。'
 const names=new Set<string>()
 const reserved=new Set(['host','cookie','set-cookie','content-length','content-type','connection','keep-alive','te','trailer','transfer-encoding','upgrade','forwarded','expect','range','accept-encoding','origin','referer','user-agent'])
 for(const [name,entry] of entries){
  const lower=name.toLowerCase()
  if(!/^[!#$%&'*+\-.^_`|~0-9a-z]+$/i.test(name)||name.length>128||reserved.has(lower)||lower.startsWith('proxy-')||lower.startsWith('x-forwarded-')||lower.startsWith('sec-'))return '仅填写鉴权头；Host、Cookie、代理、转发及传输控制头不允许使用。Cookie请使用独立输入框。'
  if(names.has(lower))return '鉴权头名称不能重复，大小写不同也视为同一个头。'
  names.add(lower)
  if(typeof entry!=='string'||entry.trim()===''||new TextEncoder().encode(entry).length>8192||/[\x00-\x08\x0a-\x1f\x7f]/.test(entry))return '每项鉴权头必须是非空字符串，不能包含换行或控制字符。'
 }
 return ''
}
