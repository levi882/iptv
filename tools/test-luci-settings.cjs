'use strict';

const fs = require('fs');

const sourcePath = process.argv[2];
if (!sourcePath) {
	console.error('Usage: node test-luci-settings.cjs settings.js');
	process.exit(2);
}

class Option {
	value() {}
}

class Section {
	tab() {}
	taboption() { return new Option(); }
}

class Map {
	section() { return new Section(); }
	render() { return {}; }
}

const form = {
	Map,
	NamedSection: class {},
	Flag: class {},
	Value: class {},
	DynamicList: class {}
};
const uci = { load: () => Promise.resolve() };
const widgets = { DeviceSelect: class {} };
const view = { extend: value => value };
const translate = value => value;

const source = fs.readFileSync(sourcePath, 'utf8');
const loadView = new Function('view', 'form', 'uci', 'widgets', '_', source);
const app = loadView(view, form, uci, widgets, translate);

Promise.resolve(app.load()).then(() => {
	app.render();
	console.log('LuCI settings render test passed');
}).catch(error => {
	console.error(error);
	process.exitCode = 1;
});
