import {expect,it} from 'vitest';
import {readdirSync,readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {numberControlsViolations} from './number-controls';
const root=resolve(__dirname,'..');
function vueFiles(dir:string):string[]{return readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?vueFiles(resolve(dir,e.name)):e.name.endsWith('.vue')?[resolve(dir,e.name)]:[])}
it('all Vue input-number controls explicitly disable step buttons',()=>{
 const violations=vueFiles(root).flatMap(file=>numberControlsViolations(readFileSync(file,'utf8')).map(line=>`${file.slice(root.length+1)}:${line}`));expect(violations).toEqual([]);
});
it.each(['<el-input-number/>','<ElInputNumber controls="false"/>','<el-input-number :controls="enabled"/>','<el-input-number :controls="true"/>','<el-input-number :[controls]="false"/>','<el-input-number :controls.prop="false"/>','<el-input-number :controls="false" v-bind="props"/>','<el-input-number :controls="false" :controls="true"/>'])('rejects a stepper regression: %s',tag=>{expect(numberControlsViolations(`<template>${tag}</template>`)).not.toEqual([])});
it('accepts only an effective explicit false and ignores comments',()=>{expect(numberControlsViolations('<template><!-- <el-input-number/> --><ElInputNumber\n :controls="false"/><el-input-number v-bind="props" :controls="false"/></template>')).toEqual([])});
it('native numbers hide browser spin UI without dropping semantic type/ranges',()=>{const css=readFileSync(resolve(root,'ui/consistency.css'),'utf8');expect(css).toContain('input[type="number"]{appearance:textfield');expect(css).toContain('::-webkit-inner-spin-button');expect(css).toContain('::-webkit-outer-spin-button');expect(css).toContain('-webkit-appearance:none')});
