/** Local-only image draft preview. data: is already allowed by the app CSP. */
export function fileDataURL(file:File):Promise<string>{return new Promise((resolve,reject)=>{const reader=new FileReader();reader.onload=()=>resolve(String(reader.result));reader.onerror=()=>reject(reader.error||new Error('无法读取本地图片'));reader.onabort=()=>reject(new Error('本地图片读取已取消'));reader.readAsDataURL(file)})}
