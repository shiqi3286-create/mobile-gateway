// paren_scan.js —— 逐行统计括号平衡，定位不平衡所在行。
const fs = require('fs');
const file = process.argv[2];
const lines = fs.readFileSync(file, 'utf8').split(/\r?\n/);
let depth = 0;
let inBlockComment = false;
lines.forEach((raw, i) => {
  let l = raw;
  // 去块注释
  if (inBlockComment) {
    const end = l.indexOf('*/');
    if (end === -1) return;
    l = l.slice(end + 2);
    inBlockComment = false;
  }
  l = l.replace(/\/\*[\s\S]*?\*\//g, ' ');
  if (l.includes('/*')) { inBlockComment = true; l = l.split('/*')[0]; }
  l = l.replace(/\/\/.*$/, '');
  l = l.replace(/`[^`]*`/g, '');
  l = l.replace(/"(?:\\.|[^"\\])*"/g, '');
  l = l.replace(/'(?:\\.|[^'\\])*'/g, '');
  let delta = 0;
  for (const ch of l) {
    if (ch === '(') delta++;
    if (ch === ')') delta--;
  }
  depth += delta;
  if (delta !== 0) console.log(String(i + 1).padStart(4), 'Δ' + (delta > 0 ? '+' : '') + delta, 'depth=' + depth, '|', raw.trim().slice(0, 90));
});
console.log('FINAL DEPTH =', depth);
