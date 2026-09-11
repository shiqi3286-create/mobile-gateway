// fix_imports.js —— 将源码中 "网关后端/internal/"（历史中文模块名）替换为 ASCII 模块名 "mobile-gateway/internal/"
const fs = require('fs');
const path = require('path');

function walk(dir, out) {
  for (const f of fs.readdirSync(dir)) {
    const full = path.join(dir, f);
    if (fs.statSync(full).isDirectory()) walk(full, out);
    else if (f.endsWith('.go')) out.push(full);
  }
  return out;
}

const files = [...walk('internal', []), 'main.go'];
for (const f of files) {
  let c = fs.readFileSync(f, 'utf8');
  const before = c;
  c = c.replace(/网关后端\/internal\//g, 'mobile-gateway/internal/');
  if (c !== before) {
    fs.writeFileSync(f, c, 'utf8');
    console.log('fixed:', f);
  }
}
console.log('done');
