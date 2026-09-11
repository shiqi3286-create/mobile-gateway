#!/usr/bin/env node
// verify_struct.js —— 校验 Go 后端 JSON 结构与前端 UI 字段的一致性。
// 用法: node verify_struct.js <adminHtmlPath> <phoneHtmlPath>
// 说明：从 HTML 中提取前端引用的字段名，与后端 stats JSON 标签做比对（启发式）。
const fs = require('fs');
const path = require('path');

const [,, adminHtml, phoneHtml] = process.argv;
if (!adminHtml || !phoneHtml) {
  console.error('用法: node verify_struct.js <adminHtml> <phoneHtml>');
  process.exit(1);
}

const admin = fs.readFileSync(adminHtml, 'utf8');
const phone = fs.readFileSync(phoneHtml, 'utf8');

// 收集前端 JS 中出现的 "xxx": "yyy" 与 json 相关字段
const fieldRegex = /["']([a-zA-Z_][a-zA-Z0-9_]*)["']\s*[:=]/g;
const fields = new Set();
for (const m of admin.matchAll(fieldRegex)) fields.add(m[1]);
for (const m of phone.matchAll(fieldRegex)) fields.add(m[1]);

// 后端 JSON 标签（从 internal 目录读取）
const goFiles = [];
function walk(dir) {
  for (const f of fs.readdirSync(dir)) {
    const full = path.join(dir, f);
    if (fs.statSync(full).isDirectory()) walk(full);
    else if (f.endsWith('.go')) goFiles.push(full);
  }
}
walk('internal');

const backendTags = new Set();
for (const f of goFiles) {
  const c = fs.readFileSync(f, 'utf8');
  for (const m of c.matchAll(/json:"([^"]+)"/g)) {
    for (const tag of m[1].split(',')) {
      if (tag) backendTags.add(tag);
    }
  }
}

// 找出前端用到但后端没有的字段（重点检查 stats 相关）
const frontendFields = new Set([...fields].filter(f => !['data','id','class','type','value','title','placeholder','style','onclick','oninput','onchange'].includes(f)));
const missing = [...frontendFields].filter(f => !backendTags.has(f));

console.log('=== 前端字段总数:', frontendFields.size, ' 后端 JSON 标签总数:', backendTags.size, '===');
console.log('=== 前端使用但后端标签缺失的字段（前 80 个）===');
console.log(missing.slice(0, 80).join('\n'));
console.log('=== 缺失总数:', missing.length);
