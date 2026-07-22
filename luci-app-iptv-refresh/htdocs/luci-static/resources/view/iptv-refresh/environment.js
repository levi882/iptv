'use strict';
'require view';
'require form';
'require fs';
'require rpc';
'require ui';
'require uci';

var DEFAULT_ENV_FILE = '/etc/iptv-refresh/provider.env';
var ACTION_HELPER = '/usr/libexec/iptv-refresh-luci-action';
var SERVICE = 'iptv-refresh';

var ENV_KEYS = [
	'MODE', 'OUTPUT_FORMAT', 'OUTPUT_PATH', 'R2H_SNAPSHOT_OUTPUT_PATH',
	'SORT_BY', 'ORDER_REF', 'KEEP_UNMATCHED', 'USE_CACHE',
	'DISPLAY_NAME_MODE', 'GROUP_BY_51ZMT', 'LINE_TAG_RULE', 'LINE_TAG_UHD',
	'LINE_TAG_HD', 'LINE_TAG_SD', 'R2H_BASE_URL', 'R2H_TOKEN',
	'R2H_IGMP_PATH', 'R2H_ADD_FCC', 'R2H_FCC_TYPE', 'R2H_PROXY_RTSP',
	'R2H_CATCHUP_HOST', 'CATCHUP_TYPE', 'CATCHUP_PLAYSEEK_TEMPLATE',
	'CATCHUP_SEEK_OFFSET', 'IGMP_HTTP_PREFIX', 'EPG_URL', 'EPG_FILE',
	'EPG_PUBLIC_FILE', 'EPG_COMPARE_SOURCE', 'EPG_REPLACE_NAME', 'X_TVG_URL',
	'LOGO_MATCH_SOURCE', 'LOGO_URL_BASE', 'LOGO_OVERRIDES_FILE',
	'LOGO_MATCH_THRESHOLD', 'LOCAL_LOGO_CACHE', 'LOCAL_LOGO_DIR',
	'LOCAL_LOGO_URL_BASE', 'LOCAL_LOGO_TIMEOUT', 'CAPTURE_TIMEOUT', 'REFRESH_TIMEOUT', 'DUMP_PATH',
	'PROVIDER_TOKEN_SERVER', 'PROVIDER_PLATFORM_ORIGIN', 'PROVIDER_EPG_ENTRY',
	'PROVIDER_EPG_ENTRY_FALLBACKS', 'PROVIDER_EASIP', 'PROVIDER_NETWORKID', 'PROVIDER_CITYCODE',
	'PROVIDER_STB_TYPE', 'PROVIDER_PRMID', 'PROVIDER_DRM_SUPPLIER',
	'PROVIDER_BIND_INTERFACE', 'PROVIDER_BIND_SOURCE_IP', 'PROVIDER_USER_AGENT', 'PROVIDER_TIMEOUT'
];

var LEGACY_ENV_KEYS = {
	HB_TOKEN_SERVER: 'PROVIDER_TOKEN_SERVER',
	HB_PLATFORM_ORIGIN: 'PROVIDER_PLATFORM_ORIGIN',
	HB_EPG_ENTRY: 'PROVIDER_EPG_ENTRY',
	HB_EPG_ENTRY_FALLBACKS: 'PROVIDER_EPG_ENTRY_FALLBACKS',
	HB_EASIP: 'PROVIDER_EASIP',
	HB_NETWORKID: 'PROVIDER_NETWORKID',
	HB_CITYCODE: 'PROVIDER_CITYCODE',
	HB_STB_TYPE: 'PROVIDER_STB_TYPE',
	HB_PRMID: 'PROVIDER_PRMID',
	HB_DRM_SUPPLIER: 'PROVIDER_DRM_SUPPLIER',
	HB_BIND_INTERFACE: 'PROVIDER_BIND_INTERFACE',
	HB_BIND_SOURCE_IP: 'PROVIDER_BIND_SOURCE_IP',
	HB_USER_AGENT: 'PROVIDER_USER_AGENT',
	HB_TIMEOUT: 'PROVIDER_TIMEOUT'
};

var DEFAULTS = {
	MODE: 'auto',
	OUTPUT_FORMAT: 'm3u',
	OUTPUT_PATH: '/mnt/iptv/iptv-refresh/config/local/local_stb.m3u',
	R2H_SNAPSHOT_OUTPUT_PATH: '/mnt/iptv/iptv-refresh/config/local/local_stb.snapshot.m3u',
	SORT_BY: 'user_channel_id',
	KEEP_UNMATCHED: 'append',
	USE_CACHE: '1',
	DISPLAY_NAME_MODE: 'tvg_name',
	GROUP_BY_51ZMT: '1',
	LINE_TAG_RULE: 'hd_sd',
	LINE_TAG_UHD: '超高清',
	LINE_TAG_HD: '高清',
	LINE_TAG_SD: '标清',
	R2H_BASE_URL: 'auto',
	R2H_IGMP_PATH: 'udp',
	R2H_ADD_FCC: '1',
	R2H_FCC_TYPE: 'telecom',
	R2H_PROXY_RTSP: '1',
	R2H_CATCHUP_HOST: 'auto',
	CATCHUP_TYPE: 'shift',
	CATCHUP_PLAYSEEK_TEMPLATE: '{(b)YmdHMS}-{(e)YmdHMS}',
	CATCHUP_SEEK_OFFSET: '-900',
	EPG_URL: 'http://epg.51zmt.top:8000/e.xml.gz',
	EPG_FILE: '/mnt/iptv/iptv-refresh/cache/e1.xml.gz',
	EPG_PUBLIC_FILE: '/www/iptv_epg/e1.xml.gz',
	X_TVG_URL: 'auto',
	LOGO_MATCH_SOURCE: 'https://live.fanmingming.com/tv/m3u/index.m3u',
	LOGO_MATCH_THRESHOLD: '0.65',
	LOCAL_LOGO_CACHE: '1',
	LOCAL_LOGO_DIR: '/www/iptv_logo',
	LOCAL_LOGO_URL_BASE: 'auto',
	LOCAL_LOGO_TIMEOUT: '20',
	CAPTURE_TIMEOUT: '180',
	REFRESH_TIMEOUT: '300',
	DUMP_PATH: '',
	PROVIDER_TOKEN_SERVER: 'auto',
	PROVIDER_PLATFORM_ORIGIN: 'auto',
	PROVIDER_EPG_ENTRY: 'auto',
	PROVIDER_EASIP: 'auto',
	PROVIDER_NETWORKID: 'auto',
	PROVIDER_STB_TYPE: 'auto',
	PROVIDER_PRMID: 'auto',
	PROVIDER_DRM_SUPPLIER: 'auto',
	PROVIDER_BIND_INTERFACE: 'auto',
	PROVIDER_USER_AGENT: 'auto',
	PROVIDER_TIMEOUT: '20'
};

var callInitAction = rpc.declare({
	object: 'luci',
	method: 'setInitAction',
	params: [ 'name', 'action' ],
	expect: { result: false }
});

function decodeEnvValue(value) {
	value = String(value || '').trim();
	if (value.length >= 2 && value.charAt(0) === '"' && value.charAt(value.length - 1) === '"') {
		try {
			return JSON.parse(value);
		} catch (e) {
			return value.substring(1, value.length - 1);
		}
	}
	if (value.length >= 2 && value.charAt(0) === "'" && value.charAt(value.length - 1) === "'")
		return value.substring(1, value.length - 1);
	return value;
}

function parseEnvironment(text) {
	var values = {};
	String(text || '').replace(/\r\n?/g, '\n').split('\n').forEach(function(line) {
		var match = line.match(/^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=(.*)$/);
		if (match)
			values[match[1]] = decodeEnvValue(match[2]);
	});
	Object.keys(LEGACY_ENV_KEYS).forEach(function(legacy) {
		var current = LEGACY_ENV_KEYS[legacy];
		if (!Object.prototype.hasOwnProperty.call(values, current) && Object.prototype.hasOwnProperty.call(values, legacy))
			values[current] = values[legacy];
	});
	return values;
}

function redactEnvironment(text) {
	return String(text || '').replace(
		/^(\s*(?:export\s+)?R2H_TOKEN\s*=).*$/gm,
		'$1********'
	);
}

function encodeEnvValue(value) {
	value = value == null ? '' : String(value);
	return value === '' ? '' : JSON.stringify(value);
}

function updateEnvironment(text, values) {
	var known = {};
	var found = {};
	ENV_KEYS.forEach(function(key) { known[key] = true; });
	Object.keys(LEGACY_ENV_KEYS).forEach(function(key) { known[key] = true; });

	var lines = String(text || '').replace(/\r\n?/g, '\n').split('\n').map(function(line) {
		var match = line.match(/^(\s*(?:export\s+)?)([A-Za-z_][A-Za-z0-9_]*)\s*=.*$/);
		if (!match || !known[match[2]])
			return line;
		var key = LEGACY_ENV_KEYS[match[2]] || match[2];
		if (found[key])
			return '';
		found[key] = true;
		return match[1] + key + '=' + encodeEnvValue(values[key]);
	});

	while (lines.length && lines[lines.length - 1] === '')
		lines.pop();
	ENV_KEYS.forEach(function(key) {
		if (!found[key])
			lines.push(key + '=' + encodeEnvValue(values[key]));
	});
	return lines.join('\n') + '\n';
}

function addValue(section, tab, key, title, description, datatype, placeholder) {
	var option = section.taboption(tab, form.Value, key, title, description);
	option.rmempty = true;
	if (datatype)
		option.datatype = datatype;
	if (placeholder != null)
		option.placeholder = placeholder;
	return option;
}

function addFlag(section, tab, key, title, description, defaultValue) {
	var option = section.taboption(tab, form.Flag, key, title, description);
	option.enabled = '1';
	option.disabled = '0';
	option.default = defaultValue || '0';
	option.rmempty = false;
	return option;
}

return view.extend({
	load: function() {
		var self = this;
		return uci.load('iptv-refresh').then(function() {
			self.envFile = uci.get('iptv-refresh', 'main', 'env_file') || DEFAULT_ENV_FILE;
			return L.resolveDefault(fs.read_direct(self.envFile), '');
		});
	},

	render: function(envText) {
		var parsed = parseEnvironment(envText);
		var env = { _raw_preview: redactEnvironment(envText) };
		ENV_KEYS.forEach(function(key) {
			env[key] = Object.prototype.hasOwnProperty.call(parsed, key) ? parsed[key] : (DEFAULTS[key] || '');
		});

		var m = new form.JSONMap({ env: env }, _('IPTV Refresh environment'), _('Structured editor for playlist, rtp2httpd, EPG, logo, catch-up, and provider options. Unknown variables and comments in the provider environment file are preserved.'));
		m.readonly = !L.hasViewPermission();
		m.tabbed = true;
		var s = m.section(form.NamedSection, 'env', 'env');
		s.anonymous = true;
		s.addremove = false;
		s.hidetitle = true;
		s.tab('output', _('Output & naming'));
		s.tab('rtp2httpd', _('rtp2httpd'));
		s.tab('epg', _('EPG & logos'));
		s.tab('provider', _('Provider & capture'));
		s.tab('raw', _('Raw preview'));

		var o = s.taboption('output', form.ListValue, 'MODE', _('Stream preference'), _('Choose which provider stream URL is preferred.'));
		o.value('auto', _('Automatic'));
		o.value('rtsp', 'RTSP');
		o.value('igmp', 'IGMP');
		o.default = 'auto';

		o = s.taboption('output', form.ListValue, 'OUTPUT_FORMAT', _('Output format'));
		o.value('auto', _('Automatic'));
		o.value('m3u', 'M3U');
		o.value('txt', 'TXT');
		o.default = 'm3u';

		addValue(s, 'output', 'OUTPUT_PATH', _('Playlist output path'), null, null, DEFAULTS.OUTPUT_PATH);
		addValue(s, 'output', 'R2H_SNAPSHOT_OUTPUT_PATH', _('Snapshot playlist path'), _('Optional playlist with rtp2httpd snapshot URLs.'), null, DEFAULTS.R2H_SNAPSHOT_OUTPUT_PATH);

		o = s.taboption('output', form.ListValue, 'SORT_BY', _('Channel sorting'));
		o.value('user_channel_id', _('User channel ID'));
		o.value('input', _('Provider order'));
		o.default = 'user_channel_id';

		addValue(s, 'output', 'ORDER_REF', _('Order reference'), _('Optional local or remote playlist used as ordering reference.'));
		o = s.taboption('output', form.ListValue, 'KEEP_UNMATCHED', _('Unmatched channels'));
		o.value('append', _('Append'));
		o.value('drop', _('Drop'));
		o.default = 'append';
		addFlag(s, 'output', 'USE_CACHE', _('Use cached provider data'), null, '1');

		o = s.taboption('output', form.ListValue, 'DISPLAY_NAME_MODE', _('Displayed channel name'));
		o.value('name', _('Provider name'));
		o.value('tvg_name', _('EPG name'));
		o.default = 'tvg_name';
		addFlag(s, 'output', 'GROUP_BY_51ZMT', _('Use 51zmt groups'), _('Infer group-title values from the 51zmt reference playlist.'), '1');

		o = s.taboption('output', form.ListValue, 'LINE_TAG_RULE', _('Quality suffix handling'));
		o.value('none', _('Keep original names'));
		o.value('hd_sd', _('Append quality tags'));
		o.default = 'hd_sd';
		o = addValue(s, 'output', 'LINE_TAG_UHD', _('UHD tag'), null, null, '超高清');
		o.depends('LINE_TAG_RULE', 'hd_sd');
		o = addValue(s, 'output', 'LINE_TAG_HD', _('HD tag'), null, null, '高清');
		o.depends('LINE_TAG_RULE', 'hd_sd');
		o = addValue(s, 'output', 'LINE_TAG_SD', _('SD tag'), null, null, '标清');
		o.depends('LINE_TAG_RULE', 'hd_sd');

		addValue(s, 'rtp2httpd', 'R2H_BASE_URL', _('rtp2httpd base URL'), _('Use auto to discover the LAN address and rtp2httpd listen port, or enter an explicit URL.'), null, DEFAULTS.R2H_BASE_URL);
		o = addValue(s, 'rtp2httpd', 'R2H_TOKEN', _('R2H token'), _('Must match the R2H Token configured in luci-app-rtp2httpd.'));
		o.password = true;

		o = s.taboption('rtp2httpd', form.ListValue, 'R2H_IGMP_PATH', _('Multicast endpoint'));
		o.value('udp', '/udp/');
		o.value('rtp', '/rtp/');
		o.default = 'udp';
		addFlag(s, 'rtp2httpd', 'R2H_ADD_FCC', _('Forward FCC parameters'), _('Append provider FCC address and type to rtp2httpd URLs.'), '1');

		o = s.taboption('rtp2httpd', form.ListValue, 'R2H_FCC_TYPE', _('FCC type'));
		o.value('telecom', 'telecom');
		o.value('huawei', 'huawei');
		o.default = 'telecom';
		o.depends('R2H_ADD_FCC', '1');

		addFlag(s, 'rtp2httpd', 'R2H_PROXY_RTSP', _('Proxy RTSP streams'), _('Rewrite RTSP streams through the rtp2httpd /rtsp/ endpoint.'), '1');
		addValue(s, 'rtp2httpd', 'R2H_CATCHUP_HOST', _('Catch-up proxy host'), _('Use auto to reuse the discovered rtp2httpd address, or enter a host and port without a scheme.'), null, DEFAULTS.R2H_CATCHUP_HOST);
		addValue(s, 'rtp2httpd', 'CATCHUP_TYPE', _('M3U catch-up type'), null, null, 'shift');
		addValue(s, 'rtp2httpd', 'CATCHUP_PLAYSEEK_TEMPLATE', _('Catch-up playseek template'), null, null, DEFAULTS.CATCHUP_PLAYSEEK_TEMPLATE);
		addValue(s, 'rtp2httpd', 'CATCHUP_SEEK_OFFSET', _('Catch-up seek offset'), _('Seconds added to the rtp2httpd catch-up request.'), 'integer', '-900');
		addValue(s, 'rtp2httpd', 'IGMP_HTTP_PREFIX', _('Direct IGMP HTTP prefix'), _('Optional direct HTTP prefix used instead of rtp2httpd URL generation.'));

		addValue(s, 'epg', 'EPG_URL', _('EPG download URL'), null, null, DEFAULTS.EPG_URL);
		addValue(s, 'epg', 'EPG_FILE', _('EPG cache file'), null, null, DEFAULTS.EPG_FILE);
		addValue(s, 'epg', 'EPG_PUBLIC_FILE', _('Published EPG file'), null, null, DEFAULTS.EPG_PUBLIC_FILE);
		addValue(s, 'epg', 'X_TVG_URL', _('M3U x-tvg-url'), _('Use auto to publish the EPG file through the router LAN address.'), null, DEFAULTS.X_TVG_URL);
		addValue(s, 'epg', 'EPG_COMPARE_SOURCE', _('EPG comparison source'), _('Optional local file or URL used for channel-name matching.'));
		addFlag(s, 'epg', 'EPG_REPLACE_NAME', _('Replace provider names with EPG names'), null, '0');
		addValue(s, 'epg', 'LOGO_MATCH_SOURCE', _('Logo matching playlist'), null, null, DEFAULTS.LOGO_MATCH_SOURCE);
		addValue(s, 'epg', 'LOGO_URL_BASE', _('Logo URL base'));
		addValue(s, 'epg', 'LOGO_OVERRIDES_FILE', _('Logo overrides file'), null, null, '/mnt/iptv/iptv-refresh/config/local/logo_overrides.csv');
		addValue(s, 'epg', 'LOGO_MATCH_THRESHOLD', _('Logo match threshold'), _('Similarity from 0 to 1.'), 'ufloat', '0.65');
		addFlag(s, 'epg', 'LOCAL_LOGO_CACHE', _('Cache logos locally'), null, '1');
		o = addValue(s, 'epg', 'LOCAL_LOGO_DIR', _('Local logo directory'), null, null, DEFAULTS.LOCAL_LOGO_DIR);
		o.depends('LOCAL_LOGO_CACHE', '1');
		o = addValue(s, 'epg', 'LOCAL_LOGO_URL_BASE', _('Local logo URL base'), _('Use auto to publish cached logos through the router LAN address.'), null, DEFAULTS.LOCAL_LOGO_URL_BASE);
		o.depends('LOCAL_LOGO_CACHE', '1');
		o = addValue(s, 'epg', 'LOCAL_LOGO_TIMEOUT', _('Logo download timeout'), _('Seconds.'), 'uinteger', '20');
		o.depends('LOCAL_LOGO_CACHE', '1');

		addValue(s, 'provider', 'CAPTURE_TIMEOUT', _('Credential capture timeout'), _('Seconds to wait for STB traffic.'), 'uinteger', '180');
		addValue(s, 'provider', 'REFRESH_TIMEOUT', _('Overall refresh timeout'), _('Maximum seconds for one complete refresh, including credential capture.'), 'uinteger', '300');
		addValue(s, 'provider', 'DUMP_PATH', _('Capture dump path'), _('Optional diagnostic file containing sensitive raw provider traffic. Leave empty for normal use.'));
		addValue(s, 'provider', 'PROVIDER_TOKEN_SERVER', _('Token server'), _('Use auto to discover the value during credential capture.'), null, 'auto');
		addValue(s, 'provider', 'PROVIDER_PLATFORM_ORIGIN', _('Platform origin'), _('Use auto to prefer captured values.'), null, 'auto');
		addValue(s, 'provider', 'PROVIDER_EPG_ENTRY', _('Provider EPG entry'), _('Use auto to prefer captured values.'), null, 'auto');
		addValue(s, 'provider', 'PROVIDER_EPG_ENTRY_FALLBACKS', _('Provider EPG fallbacks'), _('Separate multiple URLs with commas, semicolons, or spaces.'));
		addValue(s, 'provider', 'PROVIDER_EASIP', _('EAS IP'), _('Use auto to prefer captured values.'), null, 'auto');
		addValue(s, 'provider', 'PROVIDER_NETWORKID', _('Network ID'), _('Use auto to prefer captured values.'), null, 'auto');
		addValue(s, 'provider', 'PROVIDER_CITYCODE', _('City code'));
		addValue(s, 'provider', 'PROVIDER_STB_TYPE', _('STB type'), _('Use auto to replay the value captured from the STB.'), null, 'auto');
		addValue(s, 'provider', 'PROVIDER_PRMID', _('STB PRMID'), _('Use auto to replay the value captured from the STB.'), null, 'auto');
		addValue(s, 'provider', 'PROVIDER_DRM_SUPPLIER', _('STB DRM supplier'), _('Use auto to replay the value captured from the STB.'), null, 'auto');
		addValue(s, 'provider', 'PROVIDER_BIND_INTERFACE', _('Provider HTTP interface override'), _('Use auto to follow the credential capture interface, none to follow the routing table, or enter a specific device name.'), null, DEFAULTS.PROVIDER_BIND_INTERFACE);
		addValue(s, 'provider', 'PROVIDER_BIND_SOURCE_IP', _('Provider source IP'));
		addValue(s, 'provider', 'PROVIDER_USER_AGENT', _('Provider User-Agent'), _('Use auto to replay the value captured from the STB.'), null, 'auto');
		addValue(s, 'provider', 'PROVIDER_TIMEOUT', _('Provider HTTP timeout'), _('Seconds.'), 'uinteger', '20');

		o = s.taboption('raw', form.TextValue, '_raw_preview', _('Current provider environment'), _('Read-only preview. Known settings are edited in the tabs above; unknown variables and comments are preserved when saving.'));
		o.rows = 30;
		o.wrap = 'off';
		o.monospace = true;
		o.readonly = true;

		this.environmentMap = m;
		this.originalEnvironment = envText || '';
		return m.render();
	},

	writeEnvironment: function(restart) {
		var self = this;
		return this.environmentMap.parse().then(function() {
			var values = self.environmentMap.data.get('json', 'env');
			var content = updateEnvironment(self.originalEnvironment, values);
			return fs.write(self.envFile || DEFAULT_ENV_FILE, content).then(function() {
				return fs.exec(ACTION_HELPER, [ 'chmod-env' ]);
			}).then(function(result) {
				if (result.code !== 0)
					throw new Error((result.stderr || result.stdout || _('Unable to secure the environment file')).trim());
				self.originalEnvironment = content;
				if (restart)
					return callInitAction(SERVICE, 'restart');
			}).then(function() {
				ui.addNotification(null, E('p', {}, restart ? _('Environment saved and service restarted.') : _('Environment saved.')), 'info');
			});
		});
	},

	handleSave: function() {
		return this.writeEnvironment(false);
	},

	handleSaveApply: function() {
		return this.writeEnvironment(true);
	},

	handleReset: function() {
		return this.environmentMap.reset();
	}
});
