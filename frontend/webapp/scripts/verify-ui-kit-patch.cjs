'use strict';

const fs = require('fs');
const path = require('path');

const root = path.join(__dirname, '..');
const pkgPath = path.join(root, 'package.json');
const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));
const uiKitDep = pkg.dependencies?.['@odigos/ui-kit'] || '';

if (typeof uiKitDep === 'string' && uiKitDep.startsWith('file:')) {
  console.log('verify-ui-kit-patch: ok (local @odigos/ui-kit via file:, patch verification skipped)');
  process.exit(0);
}

const chunk = path.join(root, 'node_modules', '@odigos', 'ui-kit', 'lib', 'chunks', 'ui-components-Dj10kYlT.js');
const containers = path.join(root, 'node_modules', '@odigos', 'ui-kit', 'lib', 'containers.js');

function die(msg) {
  console.error(msg);
  process.exit(1);
}

if (!fs.existsSync(chunk)) {
  die(`Missing ${chunk}\nRun: cd frontend/webapp && yarn install`);
}

const s = fs.readFileSync(chunk, 'utf8');
const c = fs.readFileSync(containers, 'utf8');

if (!s.includes('map(e=>yi[e]).filter(Boolean)')) {
  die(
    '@odigos/ui-kit is not patched (expected yi[e] map + filter(Boolean) for exportedSignals).\n' +
      'Run from repo: cd frontend/webapp && yarn install\n' +
      'If patches/ is missing from your clone, git pull the branch that adds patch-package fixes.',
  );
}

if (!s.includes('null!=e&&"string"==typeof e')) {
  die(
    '@odigos/ui-kit dd() monitor guard missing (string filter before toLowerCase).\n' +
      'cd frontend/webapp && yarn install',
  );
}

if (!c.includes('if(!x){ps.includes')) {
  die(
    '@odigos/ui-kit containers.js VM profiling tab patch missing.\n' +
      'cd frontend/webapp && yarn install',
  );
}

console.log('verify-ui-kit-patch: ok');
