import {expect,it} from 'vitest'
import {readFileSync,readdirSync} from 'node:fs'
import {fileURLToPath} from 'node:url'
import {join} from 'node:path'

function sources(dir:string):string[]{return readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?sources(join(dir,e.name)):/\.(ts|vue)$/.test(e.name)&&!e.name.includes('.test.')?[join(dir,e.name)]:[])}
it('locally imported Element Plus components/services carry their own lazy CSS',()=>{
 const root=fileURLToPath(new URL('..',import.meta.url))
 for(const file of sources(root)){
  const text=readFileSync(file,'utf8')
  for(const match of text.matchAll(/import\s*{([^}]+)}\s*from\s*['"]element-plus['"]/g)){
   for(const raw of match[1]!.split(',')){
    const name=raw.trim();if(!name.startsWith('El'))continue
    const style=name.slice(2).replace(/[A-Z]/g,c=>'-'+c.toLowerCase()).replace(/^-/,'')
    expect(text,`${file}: ${name} must not rely on another route rendering a global component`).toContain(`element-plus/es/components/${style}/style/css`)
   }
  }
 }
})
