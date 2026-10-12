import {ref} from 'vue'
interface InstallPrompt extends Event {prompt():Promise<void>;userChoice:Promise<{outcome:string}>}
export const pwaPrompt=ref<InstallPrompt|null>(null),pwaUpdate=ref<ServiceWorker|null>(null)
export function initializePWA(){
 window.addEventListener('beforeinstallprompt',event=>{event.preventDefault();pwaPrompt.value=event as InstallPrompt})
 window.addEventListener('appinstalled',()=>{pwaPrompt.value=null})
 if(!window.isSecureContext||!('serviceWorker'in navigator)||!import.meta.env.PROD)return
 navigator.serviceWorker.register('/sw.js',{scope:'/',updateViaCache:'none'}).then(registration=>{
  if(registration.waiting)pwaUpdate.value=registration.waiting
  registration.addEventListener('updatefound',()=>{const worker=registration.installing;worker?.addEventListener('statechange',()=>{if(worker.state==='installed'&&navigator.serviceWorker.controller)pwaUpdate.value=registration.waiting})})
 }).catch(()=>{})
}
export async function installPWA(){if(!pwaPrompt.value)return;await pwaPrompt.value.prompt();await pwaPrompt.value.userChoice;pwaPrompt.value=null}
export function activateUpdate(){if(!pwaUpdate.value)return;navigator.serviceWorker.addEventListener('controllerchange',()=>location.reload(),{once:true});pwaUpdate.value.postMessage({type:'ACTIVATE_UPDATE'})}
