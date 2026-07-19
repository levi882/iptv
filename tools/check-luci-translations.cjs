'use strict';

const fs = require('fs');

const [poPath, ...sourcePaths] = process.argv.slice(2);
if (!poPath || sourcePaths.length === 0) {
	console.error('Usage: node check-luci-translations.cjs translations.po source.js [...]');
	process.exit(2);
}

function decodeQuoted(value) {
	return JSON.parse('"' + value + '"');
}

const po = fs.readFileSync(poPath, 'utf8').replace(/\r\n?/g, '\n');
const translations = new Map();
const entryPattern = /^msgid "((?:\\.|[^"\\])*)"\nmsgstr "((?:\\.|[^"\\])*)"$/gm;
let match;
while ((match = entryPattern.exec(po)) !== null) {
	const id = decodeQuoted(match[1]);
	if (id)
		translations.set(id, decodeQuoted(match[2]));
}

const used = new Set();
const literalPattern = /_\('((?:\\.|[^'\\])*)'\)/g;
for (const sourcePath of sourcePaths) {
	const source = fs.readFileSync(sourcePath, 'utf8');
	while ((match = literalPattern.exec(source)) !== null)
		used.add(match[1].replace(/\\'/g, "'").replace(/\\\\/g, '\\'));
}

const missing = [...used].filter(id => !translations.has(id) || !translations.get(id)).sort();
if (missing.length > 0) {
	console.error('Missing Simplified Chinese LuCI translations:');
	for (const id of missing)
		console.error('- ' + id);
	process.exit(1);
}

console.log(`LuCI Simplified Chinese translations complete (${used.size} strings)`);
