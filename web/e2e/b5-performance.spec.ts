import {test,expect} from '@playwright/test'
import {appendFileSync} from 'node:fs'
import {u7Fixture} from './u7-fixture'
test('B5 cold dashboard LCP and transferred static bytes',async({page})=>{
 test.skip(process.env.MEDIA_TOOL_B5_PERF!=='1','explicit same-machine performance workload')
 if(process.env.B5_NETWORK_PROFILE==='fixed'){
  const cdp=await page.context().newCDPSession(page)
  await cdp.send('Network.enable')
  await cdp.send('Network.emulateNetworkConditions',{offline:false,latency:80,downloadThroughput:1250000,uploadThroughput:1250000})
 }
 await page.addInitScript(()=>{
  ;(window as any).__b5LCP=0
  new PerformanceObserver(list=>{for(const entry of list.getEntries())(window as any).__b5LCP=entry.startTime}).observe({type:'largest-contentful-paint',buffered:true})
 })
 await u7Fixture(page,'/dashboard')
 await page.goto('/dashboard')
 await expect(page.getByRole('heading',{name:'首页',exact:true})).toBeVisible()
 await expect(page.locator('.state-loading:visible,.el-skeleton:visible')).toHaveCount(0)
 await page.evaluate(()=>document.fonts.ready)
 await page.evaluate(()=>new Promise<void>(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve()))))
 await expect.poll(()=>page.evaluate(()=>(window as any).__b5LCP)).toBeGreaterThan(0)
 const metric=await page.evaluate(()=>{
  const staticEntries=(performance.getEntriesByType('resource') as PerformanceResourceTiming[]).filter(e=>new URL(e.name).origin===location.origin&&!new URL(e.name).pathname.startsWith('/api/'))
  const navigation=performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming
  return {lcpMs:(window as any).__b5LCP,transferBytes:staticEntries.reduce((n,e)=>n+e.transferSize,0)+navigation.transferSize,encodedBytes:staticEntries.reduce((n,e)=>n+e.encodedBodySize,0)+navigation.encodedBodySize,decodedBytes:staticEntries.reduce((n,e)=>n+e.decodedBodySize,0)+navigation.decodedBodySize,resources:staticEntries.length,domContentLoadedMs:navigation.domContentLoadedEventEnd}
 })
 expect(metric.encodedBytes).toBeGreaterThan(0)
 console.log('B5_WEB_METRIC '+JSON.stringify(metric))
 if(process.env.B5_WEB_METRICS_FILE)appendFileSync(process.env.B5_WEB_METRICS_FILE,JSON.stringify(metric)+'\n')
})
