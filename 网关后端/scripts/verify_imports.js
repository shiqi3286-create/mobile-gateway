// verify_imports.js —— 检查每个 .go 文件是否存在“导入但未使用”的包（Go 的硬编译错误）。
const fs = require('fs');
const path = require('path');

const files = [];
function walk(dir) {
  for (const f of fs.readdirSync(dir)) {
    const full = path.join(dir, f);
    if (fs.statSync(full).isDirectory()) walk(full);
    else if (f.endsWith('.go')) files.push(full);
  }
}
walk('internal');
files.push('main.go');

let errors = 0;
for (const f of files) {
  const c = fs.readFileSync(f, 'utf8');
  // 提取 import 块
  const block = c.match(/import\s*\(([\s\S]*?)\)/);
  if (!block) continue;
  const body = block[1];
  const names = [];
  for (const line of body.split(/\r?\n/)) {
    const t = line.trim();
    if (!t || t.startsWith('//')) continue;
    // 形如: alias "path"  或  "path"
    const m = t.match(/^(?:(\w+)\s+)?"([^"]+)"/);
    if (!m) continue;
    if (m[1]) { names.push(m[1]); continue; }
    const p = m[2];
    names.push(p.split('/').pop());
  }
  // 去掉 import 块后再找引用
  const rest = c.replace(block[0], '');
  for (const n of names) {
    if (n === '_' || n === '.') continue;
    const re = new RegExp(`\\b${n}\\.`);
    if (!re.test(rest)) {
      console.error(`✗ ${f}: 导入了 "${n}" 但未使用`);
      errors++;
    }
  }
}
console.log(files.length + ' 个文件检查完成' + (errors === 0 ? ' ✓' : `，发现 ${errors} 个未使用导入`));
process.exit(errors === 0 ? 0 : 1);
