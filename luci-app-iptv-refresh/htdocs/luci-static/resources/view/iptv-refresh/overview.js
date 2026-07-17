'use strict';
'require view';
'require fs';
'require poll';
'require rpc';
'require ui';

var SERVICE = 'iptv-refresh';
var READ_HELPER = '/usr/libexec/iptv-refresh-luci';
var ACTION_HELPER = '/usr/libexec/iptv-refresh-luci-action';

var callInitAction = rpc.declare({
	object: 'luci',
	method: 'setInitAction',
	params: [ 'name', 'action' ],
	expect: { result: false }
});

function helper(path, action) {
	return fs.exec(path, [ action ]).then(function(result) {
		if (result.code !== 0)
			throw new Error((result.stderr || result.stdout || _('Command failed')).trim());
		return result.stdout || '';
	});
}

function parseStatus(raw) {
	var response = JSON.parse(raw);
	if (!response || response.ok !== true || !response.status)
		throw new Error(_('The service returned an invalid status response'));
	return response.status;
}

function displayTime(value) {
	if (!value || value.indexOf('0001-01-01') === 0)
		return '-';
	var date = new Date(value);
	return isNaN(date.getTime()) ? value : date.toLocaleString();
}

function setText(id, value) {
	var element = document.getElementById(id);
	if (element)
		element.textContent = value == null || value === '' ? '-' : String(value);
}

return view.extend({
	load: function() {
		return Promise.all([
			L.resolveDefault(helper(READ_HELPER, 'status'), null),
			L.resolveDefault(helper(READ_HELPER, 'version'), _('unknown')),
			L.resolveDefault(helper(READ_HELPER, 'log'), '')
		]);
	},

	updateStatus: function(raw) {
		var status;
		try {
			status = parseStatus(raw);
		} catch (error) {
			setText('iptv-service-status', _('Stopped or unavailable'));
			setText('iptv-refresh-state', '-');
			var button = document.getElementById('iptv-refresh-button');
			if (button)
				button.disabled = true;
			return;
		}

		var report = status.report || {};
		setText('iptv-service-status', _('Running'));
		setText('iptv-refresh-state', status.running ? _('Refreshing') : _('Idle'));
		setText('iptv-started-at', displayTime(status.started_at));
		setText('iptv-finished-at', displayTime(status.finished_at));
		setText('iptv-last-error', status.last_error || '-');
		setText('iptv-channels', report.channels);
		setText('iptv-timeshift', report.timeshift);
		setText('iptv-epg-mapped', report.epg_mapped);
		setText('iptv-logos', report.logos_matched);
		setText('iptv-output', report.output_path);
		var button = document.getElementById('iptv-refresh-button');
		if (button)
			button.disabled = status.running === true;
	},

	pollStatus: function() {
		return L.resolveDefault(helper(READ_HELPER, 'status'), null).then(L.bind(function(raw) {
			this.updateStatus(raw);
		}, this));
	},

	handleServiceAction: function(action, event) {
		var button = event.currentTarget;
		button.disabled = true;
		return callInitAction(SERVICE, action).then(L.bind(function() {
			ui.addNotification(null, E('p', {}, _('Service action completed: %s').format(action)), 'info');
			return new Promise(function(resolve) { window.setTimeout(resolve, 1000); });
		}, this)).then(L.bind(this.pollStatus, this)).catch(function(error) {
			ui.addNotification(null, E('p', {}, error.message), 'error');
		}).finally(function() {
			button.disabled = false;
		});
	},

	handleRefresh: function(event) {
		var button = event.currentTarget;
		button.disabled = true;
		return helper(ACTION_HELPER, 'refresh').then(function() {
			ui.addNotification(null, E('p', {}, _('Playlist refresh started.')), 'info');
		}).catch(function(error) {
			button.disabled = false;
			ui.addNotification(null, E('p', {}, error.message), 'error');
		});
	},

	handleDownload: function(event) {
		var button = event.currentTarget;
		button.disabled = true;
		return helper(READ_HELPER, 'playlist').then(function(content) {
			var objectURL = URL.createObjectURL(new Blob([ content ], { type: 'application/vnd.apple.mpegurl;charset=utf-8' }));
			var link = E('a', { href: objectURL, download: 'playlist.m3u' });
			document.body.appendChild(link);
			link.click();
			link.remove();
			window.setTimeout(function() { URL.revokeObjectURL(objectURL); }, 1000);
		}).catch(function(error) {
			ui.addNotification(null, E('p', {}, error.message), 'error');
		}).finally(function() {
			button.disabled = false;
		});
	},

	handleReloadLog: function(event) {
		var button = event.currentTarget;
		button.disabled = true;
		return helper(READ_HELPER, 'log').then(function(log) {
			setText('iptv-log-output', log || _('No matching log entries.'));
		}).catch(function(error) {
			ui.addNotification(null, E('p', {}, error.message), 'error');
		}).finally(function() {
			button.disabled = false;
		});
	},

	render: function(data) {
		var initialStatus = data[0];
		var version = String(data[1] || '').trim();
		var log = data[2] || '';

		poll.add(L.bind(this.pollStatus, this), 3);

		var rows = [
			[ _('Service status'), 'iptv-service-status' ],
			[ _('Refresh state'), 'iptv-refresh-state' ],
			[ _('Started at'), 'iptv-started-at' ],
			[ _('Finished at'), 'iptv-finished-at' ],
			[ _('Last error'), 'iptv-last-error' ],
			[ _('Channels'), 'iptv-channels' ],
			[ _('Timeshift channels'), 'iptv-timeshift' ],
			[ _('EPG matches'), 'iptv-epg-mapped' ],
			[ _('Logo matches'), 'iptv-logos' ],
			[ _('Playlist path'), 'iptv-output' ],
			[ _('Backend version'), 'iptv-version', version || '-' ]
		];

		var table = E('table', { 'class': 'table' }, rows.map(function(row) {
			return E('tr', { 'class': 'tr' }, [
				E('td', { 'class': 'td left', 'style': 'width:35%' }, row[0]),
				E('td', { 'class': 'td left', 'id': row[1] }, row[2] || '-')
			]);
		}));

		var viewNode = E('div', { 'class': 'cbi-map' }, [
			E('h2', {}, _('IPTV Refresh')),
			E('div', { 'class': 'cbi-map-descr' }, _('Manage the local IPTV playlist refresh service. The API token remains on the router and is never sent to this browser.')),
			E('div', { 'class': 'cbi-section' }, [ E('h3', {}, _('Status')), table ]),
			E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Actions')),
				E('div', { 'class': 'cbi-section-node' }, [
					E('button', { 'class': 'btn cbi-button cbi-button-action important', 'id': 'iptv-refresh-button', 'click': ui.createHandlerFn(this, 'handleRefresh') }, _('Refresh playlist')),
					' ',
					E('button', { 'class': 'btn cbi-button cbi-button-positive', 'click': ui.createHandlerFn(this, 'handleServiceAction', 'start') }, _('Start')),
					' ',
					E('button', { 'class': 'btn cbi-button cbi-button-action', 'click': ui.createHandlerFn(this, 'handleServiceAction', 'restart') }, _('Restart')),
					' ',
					E('button', { 'class': 'btn cbi-button cbi-button-negative', 'click': ui.createHandlerFn(this, 'handleServiceAction', 'stop') }, _('Stop')),
					' ',
					E('button', { 'class': 'btn cbi-button', 'click': ui.createHandlerFn(this, 'handleDownload') }, _('Download playlist'))
				])
			]),
			E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Recent log')),
				E('button', { 'class': 'btn cbi-button', 'click': ui.createHandlerFn(this, 'handleReloadLog') }, _('Reload log')),
				E('pre', { 'id': 'iptv-log-output', 'style': 'max-height:28em;overflow:auto;white-space:pre-wrap;margin-top:1em' }, log || _('No matching log entries.'))
			])
		]);

		window.setTimeout(L.bind(function() { this.updateStatus(initialStatus); }, this), 0);
		return viewNode;
	},

	handleSaveApply: null,
	handleSave: null,
	handleReset: null
});
