'use strict';

const assert = require('assert');
const fs = require('fs');

const sourcePath = process.argv[2];
if (!sourcePath) {
	console.error('Usage: node test-luci-environment.cjs environment.js');
	process.exit(2);
}

const writes = [];
const execs = [];
const serviceActions = [];

class Option {
	value() {}
	depends() {}
}

class Section {
	tab() {}
	taboption() { return new Option(); }
}

class JSONMap {
	constructor(data) {
		this.raw = data;
		this.data = {
			get: (_config, section) => this.raw[section]
		};
	}
	section() {
		this.lastSection = new Section();
		return this.lastSection;
	}
	render() { return {}; }
	parse() { return Promise.resolve(); }
	reset() { return Promise.resolve(); }
}

const form = {
	JSONMap,
	NamedSection: class {},
	Value: class {},
	Flag: class {},
	ListValue: class {},
	TextValue: class {}
};
const luciFS = {
	read_direct: () => Promise.resolve(''),
	write: (path, content) => {
		writes.push({ path, content });
		return Promise.resolve();
	},
	exec: (path, args) => {
		execs.push({ path, args });
		return Promise.resolve({ code: 0, stdout: '', stderr: '' });
	}
};
const rpc = {
	declare: () => (...args) => {
		serviceActions.push(args);
		return Promise.resolve({ result: true });
	}
};
const ui = { addNotification() {} };
const view = { extend: value => value };
const L = {
	resolveDefault: promise => promise,
	hasViewPermission: () => true
};
const translate = value => value;
const element = () => ({});

const source = fs.readFileSync(sourcePath, 'utf8');
const loadView = new Function('view', 'form', 'fs', 'rpc', 'ui', 'L', '_', 'E', source);
const app = loadView(view, form, luciFS, rpc, ui, L, translate, element);

const original = [
	'# preserved comment',
	'MODE=auto',
	'R2H_TOKEN=secret-token',
	'UNKNOWN_OPTION=keep-me',
	''
].join('\n');

app.render(original);
assert.strictEqual(app.environmentMap.lastSection.hidetitle, true);
assert.strictEqual(app.environmentMap.raw.env.HB_BIND_INTERFACE, 'auto');
assert(!app.environmentMap.raw.env._raw_preview.includes('secret-token'));
assert(app.environmentMap.raw.env._raw_preview.includes('R2H_TOKEN=********'));

app.environmentMap.raw.env.MODE = 'igmp';
app.writeEnvironment(true).then(() => {
	assert.strictEqual(writes.length, 1);
	assert.strictEqual(writes[0].path, '/etc/iptv-refresh/hb.env');
	assert(writes[0].content.includes('# preserved comment'));
	assert(writes[0].content.includes('UNKNOWN_OPTION=keep-me'));
	assert(writes[0].content.includes('MODE="igmp"'));
	assert(writes[0].content.includes('R2H_TOKEN="secret-token"'));
	assert.deepStrictEqual(execs[0], {
		path: '/usr/libexec/iptv-refresh-luci-action',
		args: [ 'chmod-env' ]
	});
	assert.deepStrictEqual(serviceActions[0], [ 'iptv-refresh', 'restart' ]);
	console.log('LuCI environment render/save test passed');
}).catch(error => {
	console.error(error);
	process.exitCode = 1;
});
