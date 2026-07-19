'use strict';

const fs = require('fs');

const sourcePath = process.argv[2];
if (!sourcePath) {
	console.error('Usage: node test-luci-settings.cjs settings.js');
	process.exit(2);
}

class Option {
	value() {}
	depends() {}
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
if (!source.includes("'provider_iface'") || !source.includes("'Follow routing table'")) {
	throw new Error('Provider HTTP interface setting is missing');
}
if (!source.includes("'nginx_proxy'") || !source.includes("'nginx_allow_ip'")) {
	throw new Error('Home Assistant compatibility proxy settings are missing');
}
if (!source.includes("'stb_power_enabled'") || !source.includes("'ha_webhook_url'") || !source.includes("'ha_webhook_timeout'")) {
	throw new Error('Home Assistant STB power-on settings are missing');
}
if (!source.includes("o.value('127.0.0.1')")) {
	throw new Error('Home Assistant compatibility proxy is not loopback-only by default');
}
if (!source.includes("var ENV_FILE = '/etc/iptv-refresh/provider.env'") || !source.includes("o.default = 'any'") || !source.includes("o.default = 'none'")) {
	throw new Error('Provider-neutral environment path or capture-interface default is missing');
}
const loadView = new Function('view', 'form', 'uci', 'widgets', '_', source);
const app = loadView(view, form, uci, widgets, translate);

Promise.resolve(app.load()).then(() => {
	app.render();
	console.log('LuCI settings render test passed');
}).catch(error => {
	console.error(error);
	process.exitCode = 1;
});
