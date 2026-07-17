'use strict';

const fs = require('fs');

if (process.argv.length < 3) {
	console.error('Usage: node check-luci-js.cjs view.js [...]');
	process.exit(2);
}

for (const path of process.argv.slice(2)) {
	const source = fs.readFileSync(path, 'utf8');
	new Function(source);
	console.log(`OK ${path}`);
}
