import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { resolve, relative } from 'node:path'
import {parse} from '@vue/compiler-dom'
import ts from 'typescript'

const root = resolve(__dirname, '..')
const allowlistPath = resolve(__dirname, 'copywriting-allowlist.json')
const allowlist: Record<string, string[]> = JSON.parse(readFileSync(allowlistPath, 'utf8'))

const FORBIDDEN_WORDS = [
  '工程框架',
  '实施计划',
  'docs/',
  'DEVELOPMENT_PLAN',
  'SDK 契约',
  '契约 V2',
  'operationKey',
  'fingerprint',
  'Provider v1',
  '推荐 Go',
  'Codex',
  'Claude',
  'Q5',
  'TODO',
  'WORKSPACE',
  '流水线中', '过期租约', '主任务执行器', '每组最多展示当前读取到的近期事项', '状态由服务检测和已保存配置推导',
  '仅展示真实服务数据',
  '读取当前记录', '上一页实例', '下一页实例', '上一页目录', '下一页目录', '上一页运行', '下一页运行',
  '刷新插件状态', '重新读取媒体', '严格302直链', '期望停用', '期望启用', '清理聚合记录', '从记录加入白名单', '带入搜索', '严格 302 直链', '保留可核对证据', '单批1–20项'
]

function collectVueFiles(dir: string): string[] {
  const results: string[] = []
  for (const entry of readdirSync(dir)) {
    const full = resolve(dir, entry)
    const st = statSync(full)
    if (st.isDirectory()) {
      results.push(...collectVueFiles(full))
    } else if (entry.endsWith('.vue')) {
      results.push(full)
    }
  }
  return results
}

describe('Copywriting Guard (Forbidden Developer Jargon)', () => {
  const vueFiles = collectVueFiles(root)

  it('scans all .vue templates for forbidden words and respects allowlist', () => {
    expect(allowlist).toEqual({})
    const violations: Array<{ file: string; word: string; line: number; text: string }> = []

    for (const filePath of vueFiles) {
      const relPath = 'web/src/' + relative(root, filePath)
      const allowedWords = allowlist[relPath] || []
      const content = readFileSync(filePath, 'utf8')

      // Main template includes nested slot templates; never inspect executable bindings as copy.
      const start=content.indexOf('<template>'),end=content.lastIndexOf('</template>')
      const templateMatch=start>=0&&end>start?[null,content.slice(start+10,end)]:null
      if (!templateMatch) continue

      const templateContent=String(templateMatch[1]);
      const ast=parse(templateContent);
      function check(text:string,line:number){for(const word of FORBIDDEN_WORDS)if(text.includes(word)&&!allowedWords.includes(word))violations.push({file:relPath,word,line,text:text.trim()})}
      function expression(text:string,line:number){const tree=ts.createSourceFile('binding.ts',text,ts.ScriptTarget.Latest,true);function visit(n:ts.Node){if(ts.isStringLiteral(n)||ts.isNoSubstitutionTemplateLiteral(n))check(n.text,line);ts.forEachChild(n,visit)}visit(tree)}
      function visit(node:any,technical=false){
        const nested=technical||node.type===1&&node.tag==='pre'&&node.props.some((p:any)=>p.name==='class'&&/technical-json|maintenance-json/.test(p.value?.content||''));
        if(nested)return;
        if(node.type===2)check(node.content,node.loc.start.line);
        if(node.type===5)expression(node.content.content,node.loc.start.line);
        for(const p of node.props||[]){if(p.type===6&&p.value&&['title','placeholder','aria-label','label','description'].includes(p.name))check(p.value.content,p.loc.start.line);if(p.type===7&&p.name!=='on'&&p.exp)expression(p.exp.content,p.loc.start.line)}
        for(const child of node.children||[])visit(child,nested);
      }
      visit(ast);
    }

    if (violations.length > 0) {
      console.error('Found forbidden copywriting violations:', violations)
    }
    expect(violations).toEqual([])
  })
})

it('approved search copy describes click-to-search all enabled sites',()=>{
 const view=readFileSync(resolve(root,'views/SearchWorkspaceView.vue'),'utf8');
 expect(view).toContain('description="点击作品即搜索全部已启用站点；投递前核对身份与做种要求。"');
 expect(view).not.toContain('先确认作品，再选择站点');
});

it('I35 approved identity section headings retain the existing confirmation and file-safety copy',()=>{
 const view=readFileSync(resolve(root,'organize/IdentityEditor.vue'),'utf8');
 for(const title of ['1 找到正确作品','2 核对季集','3 预览输出'])expect(view).toContain('<h3>'+title+'</h3>');
 expect(view).toContain('仅预览，不写入文件；原媒体与固定链接保持不变。');expect(view).toContain('不移动云端源文件、不改变文件播放链接，不覆盖手工内容。');expect(view).toContain('确认结果需核对输出与记录，不重复提交同一确认。');
});
