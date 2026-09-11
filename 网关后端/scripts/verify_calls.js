// verify_calls.js —— 模拟本地校验：核对 httpapi 中对各模块的方法调用是否存在。
// 原理：从各模块源码提取 `func (recv *Type) Name(` 得到方法名集合，
// 再扫描 httpapi 目录里的 h.Config.X / h.Logs.X / h.Stats.X 调用，检查名字是否在集合内。
// 这是名字级检查（不做类型推导），但能抓住绝大多数漏改/改名导致的编译错误。
const fs = require('fs');
const path = require('path');

function methodsOf(file, typeName) {
  const c = fs.readFileSync(file, 'utf8');
  const re = new RegExp(`func \\([^)]*\\*?${typeName}\\)\\s+(\\w+)\\s*\\(`, 'g');
  const set = new Set();
  for (const m of c.matchAll(re)) set.add(m[1]);
  return set;
}

const configMethods = methodsOf('internal/config/config.go', 'Manager');
const logsMethods = methodsOf('internal/logs/logs.go', 'Store');
const statsMethods = methodsOf('internal/stats/stats.go', 'Collector');

// 收集 httpapi 下所有 h.Config./h.Logs./h.Stats. 调用
const calls = { Config: new Set(), Logs: new Set(), Stats: new Set() };
function walk(dir) {
  for (const f of fs.readdirSync(dir)) {
    const full = path.join(dir, f);
    if (fs.statSync(full).isDirectory()) walk(full);
    else if (f.endsWith('.go')) {
      const c = fs.readFileSync(full, 'utf8');
      for (const m of c.matchAll(/h\.(Config|Logs|Stats)\.(\w+)\s*\(/g)) {
        calls[m[1]].add(m[2]);
      }
    }
  }
}
walk('internal/httpapi');

let errors = 0;
const check = (label, used, defs) => {
  for (const name of used) {
    if (!defs.has(name)) {
      console.error(`✗ httpapi 调用了 ${label}.${name}()，但该模块未定义此方法`);
      errors++;
    }
  }
};
check('Config', calls.Config, configMethods);
check('Logs', calls.Logs, logsMethods);
check('Stats', calls.Stats, statsMethods);

console.log(`Config 方法: ${[...configMethods].sort().join(', ')}`);
console.log(`Logs   方法: ${[...logsMethods].sort().join(', ')}`);
console.log(`Stats  方法: ${[...statsMethods].sort().join(', ')}`);
console.log('调用检查完成' + (errors === 0 ? ' ✓' : `，发现 ${errors} 个问题`));
process.exit(errors === 0 ? 0 : 1);
