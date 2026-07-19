'use strict';

const assert = require('assert');
const fs = require('fs');

const sourcePath = process.argv[2];
if (!sourcePath) {
	console.error('Usage: node test-luci-overview.cjs overview.js');
	process.exit(2);
}

const source = fs.readFileSync(sourcePath, 'utf8');

assert(source.includes("helper(ACTION_HELPER, 'clear-log')"));
assert(source.includes("helper(ACTION_HELPER, 'set-log-max-size', [ String(value), unit ])"));
assert(source.includes("helper(READ_HELPER, 'log-max-size')"));
assert(source.includes("_('Clear log')"));
assert(source.includes("_('Log size limit')"));
assert(!source.includes("'value': 'G'"));
assert(source.includes("unit === 'M' ? 100 : 102400"));
assert(source.includes("'iptv-log-size-value'"));
assert(source.includes("'iptv-log-size-unit'"));

console.log('LuCI overview log controls test passed');
