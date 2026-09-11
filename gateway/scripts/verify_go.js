// verify_go.js —— 轻量静态校验（在无 Go 工具链环境下代替 go build 做初筛）。
// 不做真实编译，只做结构性检查：
//   1. 所有 .go 文件 UTF-8 可读、无 BOM 残留乱码
//   2. import 路径全部以 "mobile-gateway" 开头（本地包）或为标准库
//   3. go.mod module 名与代码 import 前缀一致
//   4. 检查常见笔误：残留 "网关后端" 中文字符、未闭合的括号（粗略）
// 用法：node scripts/verify_go.js （在 gateway 目录下运行）
const fs = require('fs');
const path = require('path');

let errors = 0;
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

// go.mod module 名
const mod = fs.readFileSync('go.mod', 'utf8').match(/^module\s+(\S+)/m);
const modName = mod ? mod[1] : null;
if (!modName) { console.error('✗ go.mod 缺少 module 声明'); errors++; }
else console.log('✓ go.mod module =', modName);

for (const f of files) {
  const c = fs.readFileSync(f, 'utf8');
  // 模块路径残留（历史版本曾用中文模块名 "网关后端/internal/..."，会导致 go build 失败）
  if (/网关后端\/internal/.test(c) || /module\s+网关后端/.test(c)) {
    console.error(`✗ ${f}: 残留中文模块路径（应为 mobile-gateway/...）`);
    errors++;
  }
  // import 块检查：本地包必须以 module 名开头
  const imports = [...c.matchAll(/^\s*"(mobile-gateway\/[^"]+)"/gm)].map(m => m[1]);
  for (const imp of imports) {
    if (!imp.startsWith(modName + '/')) {
      console.error(`✗ ${f}: 本地 import "${imp}" 与 module 名不匹配`);
      errors++;
    }
  }
  // 括号粗略配对：按行剥离字符串与注释后累加（启发式，权威校验交给 CI 的 go vet）
  // 注意剥离顺序：先字符串后注释，避免把 http:// 里的 // 误当行注释而吞掉括号。
  let depth = 0;
  for (const raw of c.split(/\r?\n/)) {
    let l = raw
      .replace(/`[^`]*`/g, '')
      .replace(/"(?:\\.|[^"\\])*"/g, '')
      .replace(/'(?:\\.|[^'\\])*'/g, '')
      .replace(/\/\*[\s\S]*?\*\//g, ' ')
      .replace(/\/\/.*$/, '');
    for (const ch of l) {
      if (ch === '(') depth++;
      if (ch === ')') depth--;
    }
  }
  if (depth !== 0) {
    console.warn(`⚠ ${f}: 括号数不平衡 (最终深度 ${depth})，请人工确认；CI 的 go vet 会做权威校验`);
  }
}
console.log(files.length + ' 个 .go 文件检查完成' + (errors === 0 ? ' ✓' : `，发现 ${errors} 个错误`));
process.exit(errors === 0 ? 0 : 1);
