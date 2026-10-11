// No HTTP/media/account APIs are available in this harness. Default handlers
// are owned fixtures. The optional reference mode executes only the local,
// hash-verified callback function, never the third-party bundle or loadSource.
const fs = require('node:fs');
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const guard = fs.readFileSync(process.argv[2], 'utf8');

let binding;
if (process.argv[3]) {
  const original = fs.readFileSync(process.argv[3], 'utf8');
  assert.equal(crypto.createHash('sha256').update(original).digest('hex'),
    '8c61400307b6a13a668731bbc086b5673538f9aaee261e7b8d2bb49403a6e531');
  const start = original.indexOf('function bindEventsToHlsPlayer(');
  const end = original.indexOf('function handleHlsJsMediaError(', start);
  assert(start >= 0 && end > start);
  binding = original.slice(start, end);
  const entry = 'function bindEventsToHlsPlayer(instance,hls,elem,signal,resolve,reject){hls.on';
  const error = 'else{if(console.log("HLS Error: Type: "+data.type';
  assert.equal(binding.split(entry).length, 2);
  assert.equal(binding.split(error).length, 2);
  binding = binding.replace(entry,
    'function bindEventsToHlsPlayer(instance,hls,elem,signal,resolve,reject){' + guard + 'hls.on')
    .replace(error, 'else{if(mtStartupGuard.shouldWait(data))return;if(console.log("HLS Error: Type: "+data.type');
} else {
  binding = `function bindEventsToHlsPlayer(instance,hls,elem,signal,resolve,reject){
${guard}
hls.on(Hls.Events.MANIFEST_PARSED,function(){
  instance.playWithPromise(elem,signal).then(function(value){
    if(signal && signal.aborted){destroyHlsPlayer(hls);if(reject){reject(getAbortError(signal));reject=null;}}
    else {reject=null;if(resolve)resolve(value);}
  },function(error){if(reject){reject(error);reject=null;}});
});
hls.on(Hls.Events.ERROR,function(event,data){
  if(signal && signal.aborted){destroyHlsPlayer(hls);if(reject){reject(getAbortError(signal));reject=null;}return;}
  if(mtStartupGuard.shouldWait(data))return;
  if(data.type===Hls.ErrorTypes.NETWORK_ERROR && data.response && data.response.code>=400){
    destroyHlsPlayer(hls);if(reject){rejectWithError(0,reject,'servererror');reject=null;}else instance.onError('servererror');return;
  }
  if(data.fatal || reject){
    if(data.type===Hls.ErrorTypes.NETWORK_ERROR){
      if(reject || (data.response && data.response.code===0)){
        destroyHlsPlayer(hls);if(reject){rejectWithError(0,reject,'network');reject=null;}else instance.onError('network');
      }else hls.startLoad();
    }else if(data.type===Hls.ErrorTypes.MEDIA_ERROR){
      const current=reject;reject=null;
      if(current){destroyHlsPlayer(hls);rejectWithError(0,current,'mediadecodeerror');}else handleHlsJsMediaError(instance,null);
    }else{destroyHlsPlayer(hls);if(reject){rejectWithError(0,reject,'mediadecodeerror');reject=null;}else instance.onError('mediadecodeerror');}
  }
});
}`;
}
const Hls = { version: '1.6.0-beta.2', Events: { MANIFEST_PARSED: 'manifest', ERROR: 'error', DESTROYING: 'destroying' },
  ErrorTypes: { NETWORK_ERROR: 'networkError', MEDIA_ERROR: 'mediaError' } };
const factory = new Function('Hls','console','setTimeout','clearTimeout','destroyHlsPlayer',
  'getAbortError','rejectWithError','handleHlsJsMediaError', binding + ';return bindEventsToHlsPlayer;');
function setup({ aborted = false, playError = null } = {}) {
  let clock = 0, timerId = 0;
  const timers = new Map(), callbacks = new Map(), abortListeners = new Set();
  const result = { resolved: [], rejected: [], runtimeErrors: [], destroyed: 0, manualStarts: 0, waits: 0 };
  const controller = new AbortController();
  const signal = controller.signal;
  const nativeAdd = signal.addEventListener.bind(signal), nativeRemove = signal.removeEventListener.bind(signal);
  signal.addEventListener = (type, handler, options) => { if(type==='abort')abortListeners.add(handler);nativeAdd(type,handler,options); };
  signal.removeEventListener = (type, handler, options) => { if(type==='abort')abortListeners.delete(handler);nativeRemove(type,handler,options); };
  if(aborted)controller.abort();
  const hls = {
    on(type,handler){let set=callbacks.get(type);if(!set){set=new Set();callbacks.set(type,set);}set.add(handler);},
    off(type,handler){callbacks.get(type)?.delete(handler);},
    emit(type,data){for(const handler of [...(callbacks.get(type)||[])])handler(type,data);},
    destroy(){result.destroyed++;this.emit(Hls.Events.DESTROYING);callbacks.clear();},
    startLoad(){result.manualStarts++;}
  };
  const instance = { playWithPromise(){return playError ? Promise.reject(playError) : Promise.resolve('played');}, onError(error){result.runtimeErrors.push(error);} };
  const bind = factory(Hls, {log(message){if(String(message).startsWith('MediaTool'))result.waits++;}},
    (handler, delay) => {const id=++timerId;timers.set(id,{handler,at:clock+delay});return id;},
    id => timers.delete(id), hls => hls.destroy(), signal => {try{signal.throwIfAborted();}catch(error){return error;}},
    (_unused,reject,code) => {const error=new Error('Playback failure: '+code);error.name=code;reject(error);},
    instance => instance.onError('mediadecodeerror'));
  bind(instance,hls,{},signal,value=>result.resolved.push(value),error=>result.rejected.push(error));
  return { hls, controller, timers, abortListeners, result,
    advance(ms){clock+=ms;for(const[id,timer]of[...timers])if(timer.at<=clock){timers.delete(id);timer.handler();}} };
}
const network = (overrides={}) => ({type:Hls.ErrorTypes.NETWORK_ERROR,details:'fragLoadError',fatal:false,
  response:{code:500},errorAction:{action:5,retryConfig:{maxNumRetry:6,retryDelayMs:1000,maxRetryDelayMs:8000},retryCount:0},...overrides});
const flush = async()=>{for(let i=0;i<4;i++)await Promise.resolve();};
function settledClean(test){assert.equal(test.timers.size,0);assert.equal(test.abortListeners.size,0);}
async function run(){
  let tested=0;
  for(const code of [undefined,null,0,408,500,502,503,504]){
    const test=setup(),error=network({response:code===undefined?undefined:{code}}),before=JSON.stringify(error.errorAction);
    test.hls.emit(Hls.Events.ERROR,error);
    assert.equal(test.result.rejected.length,0);assert.equal(test.result.destroyed,0);assert.equal(test.result.manualStarts,0);
    assert.equal(JSON.stringify(error.errorAction),before);assert.equal(test.result.waits,1);
    test.hls.emit(Hls.Events.MANIFEST_PARSED);await flush();assert.deepEqual(test.result.resolved,['played']);settledClean(test);tested++;
  }
  for(const code of [400,401,403,404,409,429,501,'500']){
    const test=setup();test.hls.emit(Hls.Events.ERROR,network({response:{code}}));await flush();
    assert.equal(test.result.waits,0);assert.equal(test.result.rejected[0]?.name,'servererror');assert.equal(test.result.destroyed,1);settledClean(test);tested++;
  }
  for(const overrides of [{fatal:true},{fatal:undefined},{errorAction:undefined},
    {errorAction:{action:5,retryConfig:{maxNumRetry:0},retryCount:0}},
    {errorAction:{action:5,retryConfig:{maxNumRetry:6},retryCount:6}},
    {errorAction:{action:5,retryConfig:{maxNumRetry:6},retryCount:-1}},
    {errorAction:{action:5,retryConfig:{maxNumRetry:6},retryCount:'0'}},
    {errorAction:{action:2,retryConfig:{maxNumRetry:6},retryCount:0}},
    {errorAction:{action:0,retryConfig:{maxNumRetry:6},retryCount:0}},
    {details:'unknown-network-detail'}]){
    const test=setup();test.hls.emit(Hls.Events.ERROR,network(overrides));await flush();
    assert.equal(test.result.waits,0);assert.equal(test.result.rejected.length,1);assert.equal(test.result.destroyed,1);settledClean(test);tested++;
  }
  for(const event of [network({fatal:true,response:{code:0}}),{type:Hls.ErrorTypes.MEDIA_ERROR,fatal:false,details:'bufferStalledError'}]){
    const test=setup();test.hls.emit(Hls.Events.ERROR,event);await flush();
    assert.equal(test.result.rejected.length,1);assert.equal(test.result.destroyed,1);assert.notEqual(test.result.rejected[0].name,'AbortError');settledClean(test);tested++;
  }
  Hls.version='unknown-library';let unknown=setup();unknown.hls.emit(Hls.Events.ERROR,network());await flush();assert.equal(unknown.result.waits,0);assert.equal(unknown.result.rejected.length,1);settledClean(unknown);tested++;Hls.version='1.6.0-beta.2';
  let test=setup();test.hls.emit(Hls.Events.ERROR,network());test.advance(89999);assert.equal(test.result.rejected.length,0);test.advance(1);await flush();
  assert.equal(test.result.rejected[0]?.name,'network');assert.equal(test.result.destroyed,1);assert.equal(test.result.manualStarts,0);settledClean(test);tested++;
  test=setup({aborted:true});await flush();assert.equal(test.result.rejected[0]?.name,'AbortError');assert.equal(test.result.destroyed,1);settledClean(test);tested++;
  test=setup();test.hls.emit(Hls.Events.ERROR,network());test.controller.abort();await flush();assert.equal(test.result.rejected[0]?.name,'AbortError');assert.equal(test.result.destroyed,1);settledClean(test);test.advance(90000);assert.equal(test.result.rejected.length,1);tested++;
  test=setup();test.hls.destroy();await flush();assert.equal(test.result.rejected[0]?.name,'AbortError');assert.equal(test.result.destroyed,1);settledClean(test);tested++;
  test=setup();test.hls.emit(Hls.Events.MANIFEST_PARSED);await flush();test.hls.emit(Hls.Events.ERROR,network());await flush();assert.deepEqual(test.result.runtimeErrors,['servererror']);assert.equal(test.result.waits,0);settledClean(test);tested++;
  const playError=new Error('owned autoplay fixture');playError.name='NotAllowedError';test=setup({playError});test.hls.emit(Hls.Events.MANIFEST_PARSED);await flush();assert.equal(test.result.rejected[0],playError);settledClean(test);tested++;
  console.log(JSON.stringify({mode:process.argv[3]?'verified-local-original-callback':'owned-handler-fixture',cases:tested,
    startupBudgetMs:90000,manualRetries:0,permissionOrFatalBypass:false,networkOrAccountsAccessed:false}));
}
run().catch(()=>{console.error('Bounded startup regression failed; raw library and private state omitted.');process.exitCode=1;});
