// patch_main.js —— 移除 main.go 中的适配器层，改为直接注入具体模块。
const fs = require('fs');
const f = 'main.go';
let c = fs.readFileSync(f, 'utf8');

// 1) Handler 构造改为具体类型
const before = `	h := &httpapi.Handler{
		Stats: col,
		Config: configAdapter{cm: cm},
		Logs:  logAdapter{ls: ls},
		// Proxy / Agg 在路由转发与聚合编排模块完成后接入
	}`;
const after = `	h := &httpapi.Handler{
		Stats:  col,
		Config: cm,
		Logs:   ls,
		// Proxy / Agg 在路由转发与聚合编排模块完成后接入
	}`;
if (!c.includes(before)) {
  console.error('✗ 未找到 Handler 构造片段，可能需要人工处理');
  process.exit(1);
}
c = c.replace(before, after);

// 2) 从适配器章节标记处截断（该章节已不再需要）
const marker = '// ---------- 适配器';
const idx = c.indexOf(marker);
if (idx === -1) {
  console.error('✗ 未找到适配器章节标记');
  process.exit(1);
}
c = c.slice(0, idx).replace(/\s*$/, '\n');

fs.writeFileSync(f, c, 'utf8');
console.log('✓ main.go 已移除适配器层');
