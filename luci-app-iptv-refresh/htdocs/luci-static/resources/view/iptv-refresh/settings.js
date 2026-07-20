'use strict';
'require view';
'require form';
'require uci';
'require tools.widgets as widgets';

var ENV_FILE = '/etc/iptv-refresh/provider.env';

function validateCronExpression(value) {
	var ranges = [ [ 0, 59 ], [ 0, 23 ], [ 1, 31 ], [ 1, 12 ], [ 0, 7 ] ];
	var fields = value.trim().split(/ +/);

	if (!/^[0-9*\/, -]+$/.test(value) || fields.length !== 5)
		return _('Enter a five-field numeric cron expression.');

	function numberInRange(number, range) {
		return /^[0-9]+$/.test(number) && +number >= range[0] && +number <= range[1];
	}

	function atomIsValid(atom, range) {
		var slash = atom.split('/');
		var dash;

		if (slash.length > 2 || (slash.length === 2 && (!numberInRange(slash[1], [ 1, 65535 ]))))
			return false;
		if (slash[0] === '*')
			return true;
		dash = slash[0].split('-');
		if (dash.length === 1)
			return numberInRange(dash[0], range);
		return dash.length === 2 && numberInRange(dash[0], range) &&
			numberInRange(dash[1], range) && +dash[0] <= +dash[1];
	}

	for (var fieldIndex = 0; fieldIndex < fields.length; fieldIndex++) {
		var atoms = fields[fieldIndex].split(',');
		for (var atomIndex = 0; atomIndex < atoms.length; atomIndex++)
			if (!atomIsValid(atoms[atomIndex], ranges[fieldIndex]))
				return _('Cron expression contains a value outside its valid range.');
	}

	return true;
}

return view.extend({
	load: function() {
		return uci.load('iptv-refresh');
	},

	render: function() {
		var m = new form.Map('iptv-refresh', _('IPTV Refresh settings'), _('Changes are applied through UCI. Service-related changes automatically reload the procd service.'));
		var s = m.section(form.NamedSection, 'main', 'service', _('Service'));
		s.anonymous = true;
		s.addremove = false;
		s.tab('general', _('General'));
		s.tab('paths', _('Paths'));
		s.tab('access', _('Access control'));
		s.tab('automation', _('STB automation'));

		var o = s.taboption('general', form.Flag, 'enabled', _('Enable service'));
		o.default = '1';
		o.rmempty = false;

		o = s.taboption('general', widgets.DeviceSelect, 'iface', _('Credential capture interface'), _('Interface used only for STB credential capture. Choose any when the login path is uncertain, and cold-boot the STB after capture starts.'));
		o.value('any', _('All interfaces'));
		o.default = 'any';
		o.rmempty = false;
		o.noaliases = false;
		o.noinactive = false;

		o = s.taboption('general', widgets.DeviceSelect, 'provider_iface', _('Provider HTTP interface'), _('Logical interface used for provider authentication and channel downloads. Raw VLAN and any capture interfaces usually have no IPv4, so select the addressed DHCP/PPPoE IPTV interface explicitly. An explicit provider interface override under Environment takes precedence.'));
		o.value('auto', _('Follow capture interface'));
		o.value('none', _('Follow routing table'));
		o.default = 'none';
		o.rmempty = false;
		o.noaliases = false;
		o.noinactive = false;

		o = s.taboption('general', form.Value, 'listen_host', _('Listen address'));
		o.default = '127.0.0.1';
		o.datatype = 'ipaddr';
		o.rmempty = false;

		o = s.taboption('general', form.Value, 'listen_port', _('Listen port'));
		o.default = '9100';
		o.datatype = 'port';
		o.rmempty = false;

		o = s.taboption('paths', form.Value, 'repo_root', _('Data root'));
		o.default = '/mnt/iptv/iptv-refresh';
		o.rmempty = false;

		o = s.taboption('paths', form.Value, 'env_file', _('Environment file'));
		o.default = ENV_FILE;
		o.rmempty = false;

		o = s.taboption('paths', form.Value, 'creds_file', _('Captured credentials file'));
		o.default = '/etc/iptv-refresh/provider.creds.env';
		o.rmempty = false;

		o = s.taboption('paths', form.Value, 'token_file', _('API token file'));
		o.default = '/etc/iptv-refresh/token';
		o.rmempty = false;
		o.description = _('The token is managed by the backend and is not displayed in LuCI.');

		o = s.taboption('access', form.DynamicList, 'allow_ip', _('Allowed client IP addresses'), _('Only these source addresses may call status, refresh, and playlist endpoints. Keep loopback entries for LuCI integration.'));
		o.datatype = 'ipaddr';
		o.value('127.0.0.1');
		o.value('::1');
		o.rmempty = false;

		o = s.taboption('access', form.Flag, 'nginx_proxy', _('Home Assistant compatibility proxy'), _('Automatically inject the router API token, convert legacy GET refresh requests to POST, and ignore legacy query parameters.'));
		o.default = '1';
		o.rmempty = false;

		o = s.taboption('access', form.DynamicList, 'nginx_allow_ip', _('Home Assistant proxy source addresses'), _('Only these IP addresses or CIDR networks may use the token-injecting nginx refresh route. Prefer the exact Home Assistant IP when it is static.'));
		o.datatype = 'ipaddr';
		o.value('127.0.0.1');
		o.rmempty = false;
		o.depends('nginx_proxy', '1');

		o = s.taboption('automation', form.Flag, 'stb_power_enabled', _('Power on STB through Home Assistant'), _('During credential recapture, start packet capture first and then call the configured Home Assistant webhook. Normal saved-credential refreshes never call it.'));
		o.default = '0';
		o.rmempty = false;

		o = s.taboption('automation', form.Value, 'ha_webhook_url', _('Home Assistant webhook URL'), _('Use a local-only Home Assistant webhook such as http://homeassistant.lan:8123/api/webhook/your-random-id. The URL is treated as a secret and is not placed on the service command line.'));
		o.password = true;
		o.placeholder = 'http://homeassistant.lan:8123/api/webhook/...';
		o.rmempty = true;
		o.depends('stb_power_enabled', '1');

		o = s.taboption('automation', form.Value, 'ha_webhook_timeout', _('Home Assistant webhook timeout'), _('Seconds to wait for Home Assistant to accept the power-on request.'));
		o.default = '10';
		o.datatype = 'and(uinteger,min(1),max(60))';
		o.rmempty = false;
		o.depends('stb_power_enabled', '1');

		o = s.taboption('automation', form.Flag, 'restart_rtp2httpd_after_capture', _('Restart rtp2httpd after credential capture'), _('After a successful credential capture and playlist refresh, restart rtp2httpd. Normal saved-credential refreshes do not restart it.'));
		o.default = '0';
		o.rmempty = false;

		o = s.taboption('automation', form.Flag, 'capture_schedule_enabled', _('Scheduled credential capture and refresh'), _('Run credential capture followed by a full channel and EPG refresh on the router schedule. Enable the Home Assistant power-on webhook above or ensure the STB is powered on at that time.'));
		o.default = '0';
		o.rmempty = false;

		o = s.taboption('automation', form.Value, 'capture_schedule', _('Cron expression'), _('Use five numeric fields: minute, hour, day of month, month, and day of week. The router local time zone is used. For example, 30 7 * * * runs every day at 07:30.'));
		o.default = '30 7 * * *';
		o.placeholder = '30 7 * * *';
		o.rmempty = false;
		o.depends('capture_schedule_enabled', '1');
		o.validate = function(sectionId, value) {
			return validateCronExpression(value);
		};

		return m.render();
	}
});
