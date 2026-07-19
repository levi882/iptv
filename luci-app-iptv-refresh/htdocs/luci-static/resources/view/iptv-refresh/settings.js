'use strict';
'require view';
'require form';
'require uci';
'require tools.widgets as widgets';

var ENV_FILE = '/etc/iptv-refresh/hb.env';

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

		var o = s.taboption('general', form.Flag, 'enabled', _('Enable service'));
		o.default = '1';
		o.rmempty = false;

		o = s.taboption('general', widgets.DeviceSelect, 'iface', _('IPTV interface'), _('Interface used only for STB credential capture. Choose any when the login path is uncertain, and cold-boot the STB after capture starts.'));
		o.value('any', _('All interfaces'));
		o.default = 'eth3.3927';
		o.rmempty = false;
		o.noaliases = false;
		o.noinactive = false;

		o = s.taboption('general', widgets.DeviceSelect, 'provider_iface', _('Provider HTTP interface'), _('Logical interface used for provider authentication and channel downloads. Raw VLAN and any capture interfaces usually have no IPv4, so select the addressed DHCP/PPPoE IPTV interface explicitly. An explicit HB_BIND_INTERFACE value under Environment takes precedence.'));
		o.value('auto', _('Follow capture interface'));
		o.value('none', _('Follow routing table'));
		o.default = 'auto';
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
		o.default = '/mnt/sda1/iptv';
		o.rmempty = false;

		o = s.taboption('paths', form.Value, 'env_file', _('Environment file'));
		o.default = ENV_FILE;
		o.rmempty = false;

		o = s.taboption('paths', form.Value, 'creds_file', _('Captured credentials file'));
		o.default = '/etc/iptv-refresh/hb.creds.env';
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
		o.value('10.1.1.0/24');
		o.rmempty = false;
		o.depends('nginx_proxy', '1');

		return m.render();
	}
});
