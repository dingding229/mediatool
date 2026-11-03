import {expect,it} from 'vitest';
import {readFileSync} from 'node:fs';
import inventory from '../../../internal/modules/plugins/v2/library.json';
it('overview describes checked tasks without implementation terminology',()=>{
 const source=readFileSync(new URL('./InstanceDashboards.vue',import.meta.url),'utf8');
 expect(source).toContain('任务数为最近一次后台检查的任务数量，不含页面查看。');
 expect(source).not.toContain('概览任务数不是执行次数');
});
it('the pinned plugin supplies a Chinese tag explanation without changing the field',()=>{
 const fields=inventory.forms['torrent-assistant'].content;
 const tag=fields.find(f=>'model' in f&&f.model==='tag');
 expect(JSON.stringify(tag)).toContain('保留原有归属标签');
 expect(JSON.stringify(tag)).not.toContain('owner 标签');
});

it('notice display preserves lowercase worker identifiers and only translates a standalone Worker',async()=>{
 const {serviceText}=await import('../presentation/service-text');
 expect(serviceText('plugin-worker worker WorkerPool myWorker')).toBe('plugin-worker worker WorkerPool myWorker');
 expect(serviceText('Worker unavailable')).toBe('后台执行器 unavailable');
 expect(serviceText('未连接业务Worker')).toBe('后台执行器未就绪');
 expect(serviceText('Worker stopped before a confirmed send receipt; manual verification required')).toBe('通知结果尚未确认，后台执行器已停止，请先人工核对');
});
