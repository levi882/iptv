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

function helper(path, action, args) {
	return fs.exec(path, [ action ].concat(args || [])).then(function(result) {
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

function setState(id, value, state) {
	var element = document.getElementById(id);
	if (!element)
		return;

	element.textContent = value == null || value === '' ? '-' : String(value);
	if (state)
		element.setAttribute('data-state', state);
	else
		element.removeAttribute('data-state');
}

function hasTime(value) {
	return value && String(value).indexOf('0001-01-01') !== 0;
}

function setError(error) {
	var panel = document.getElementById('iptv-error-panel');
	if (!panel)
		return;

	panel.hidden = !error;
	setText('iptv-last-error', error || '-');
}

function setServiceControls(available) {
	var refresh = document.getElementById('iptv-refresh-button');
	var captureRefresh = document.getElementById('iptv-capture-refresh-button');
	var start = document.getElementById('iptv-start-button');
	var restart = document.getElementById('iptv-restart-button');
	var stop = document.getElementById('iptv-stop-button');
	var download = document.getElementById('iptv-download-button');

	if (refresh)
		refresh.hidden = !available;
	if (captureRefresh)
		captureRefresh.hidden = !available;
	if (start)
		start.hidden = available;
	if (restart)
		restart.hidden = !available;
	if (stop)
		stop.hidden = !available;
	if (download)
		download.hidden = !available;
}

function setRefreshDisabled(disabled) {
	var refresh = document.getElementById('iptv-refresh-button');
	var captureRefresh = document.getElementById('iptv-capture-refresh-button');
	if (refresh)
		refresh.disabled = disabled;
	if (captureRefresh)
		captureRefresh.disabled = disabled;
}

function parseLogSize(value) {
	var match = String(value || '').trim().match(/^([1-9][0-9]*)([KM])B?$/i);
	return match ? { value: match[1], unit: match[2].toUpperCase() } : { value: '1', unit: 'M' };
}

function maxLogSizeValue(unit) {
	return unit === 'M' ? 100 : 102400;
}

return view.extend({
	load: function() {
		return Promise.all([
			L.resolveDefault(helper(READ_HELPER, 'status'), null),
			L.resolveDefault(helper(READ_HELPER, 'version'), _('unknown')),
			L.resolveDefault(helper(READ_HELPER, 'log'), ''),
			L.resolveDefault(helper(READ_HELPER, 'log-max-size'), '1M')
		]);
	},

	updateStatus: function(raw) {
		var status;
		try {
			status = parseStatus(raw);
		} catch (error) {
			setState('iptv-refresh-result', '-', 'neutral');
			setState('iptv-service-status', _('Stopped or unavailable'), 'error');
			setText('iptv-refresh-state', '-');
			setText('iptv-started-at', '-');
			setText('iptv-finished-at', '-');
			setText('iptv-channels', '-');
			setText('iptv-timeshift', '-');
			setText('iptv-epg-mapped', '-');
			setText('iptv-logos', '-');
			setText('iptv-output', '-');
			setError('');
			setRefreshDisabled(true);
			setServiceControls(false);
			return;
		}

		var report = status.report || {};
		var lastError = status.last_error || '';
		var result = status.running
			? [ _('Refreshing'), 'working' ]
			: lastError
				? [ _('Failed'), 'error' ]
				: hasTime(status.finished_at)
					? [ _('Completed'), 'success' ]
					: [ _('Not run'), 'neutral' ];

		setState('iptv-refresh-result', result[0], result[1]);
		setState('iptv-service-status', _('Running'), 'success');
		setText('iptv-refresh-state', status.running ? _('Refreshing') : _('Idle'));
		setText('iptv-started-at', displayTime(status.started_at));
		setText('iptv-finished-at', displayTime(status.finished_at));
		setError(lastError);
		setText('iptv-channels', report.channels);
		setText('iptv-timeshift', report.timeshift);
		setText('iptv-epg-mapped', report.epg_mapped);
		setText('iptv-logos', report.logos_matched);
		setText('iptv-output', report.output_path);
		setRefreshDisabled(status.running === true);
		setServiceControls(true);
	},

	pollStatus: function() {
		return L.resolveDefault(helper(READ_HELPER, 'status'), null).then(L.bind(function(raw) {
			this.updateStatus(raw);
		}, this));
	},

	handleServiceAction: function(action, event) {
		var button = event.currentTarget;
		button.disabled = true;
		return callInitAction(SERVICE, action).then(function(result) {
			if (result !== true)
				throw new Error(_('Command failed') + ': ' + action);
			return new Promise(function(resolve) { window.setTimeout(resolve, 1000); });
		}).then(L.bind(function() {
			if (action === 'stop')
				return this.pollStatus();
			return helper(READ_HELPER, 'status').then(L.bind(function(raw) {
				this.updateStatus(raw);
			}, this));
		}, this)).then(function() {
			ui.addNotification(null, E('p', {}, _('Service action completed: %s').format(action)), 'info');
		}).catch(function(error) {
			ui.addNotification(null, E('p', {}, error.message), 'error');
		}).finally(function() {
			button.disabled = false;
		});
	},

	handleRefresh: function(action, event) {
		setRefreshDisabled(true);
		return helper(ACTION_HELPER, action).then(function() {
			var message = action === 'capture-refresh'
				? _('Credential capture and playlist refresh started. Keep the STB powered on.')
				: _('Playlist refresh started using saved credentials.');
			ui.addNotification(null, E('p', {}, message), 'info');
		}).catch(function(error) {
			setRefreshDisabled(false);
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

	handleClearLog: function(event) {
		var button = event.currentTarget;
		button.disabled = true;
		return helper(ACTION_HELPER, 'clear-log').then(function() {
			setText('iptv-log-output', _('No matching log entries.'));
			ui.addNotification(null, E('p', {}, _('IPTV Refresh log cleared.')), 'info');
		}).catch(function(error) {
			ui.addNotification(null, E('p', {}, error.message), 'error');
		}).finally(function() {
			button.disabled = false;
		});
	},

	handleLogMaxSize: function() {
		var input = document.getElementById('iptv-log-size-value');
		var select = document.getElementById('iptv-log-size-unit');
		var value = parseInt(input.value, 10);
		var unit = select.value;
		var maximum = maxLogSizeValue(unit);
		if (!isFinite(value) || value < 1)
			value = 1;
		if (value > maximum)
			value = maximum;
		input.value = String(value);
		input.max = String(maximum);
		input.disabled = true;
		select.disabled = true;
		return helper(ACTION_HELPER, 'set-log-max-size', [ String(value), unit ]).then(function() {
			return helper(READ_HELPER, 'log');
		}).then(function(log) {
			setText('iptv-log-output', log || _('No matching log entries.'));
			ui.addNotification(null, E('p', {}, _('Log size limit updated to %s %s.').format(value, unit + 'B')), 'info');
		}).catch(function(error) {
			ui.addNotification(null, E('p', {}, error.message), 'error');
		}).finally(function() {
			input.disabled = false;
			select.disabled = false;
		});
	},

	render: function(data) {
		var initialStatus = data[0];
		var version = String(data[1] || '').trim();
		var log = data[2] || '';
		var logSize = parseLogSize(data[3]);

		poll.add(L.bind(this.pollStatus, this), 3);

		var metric = function(label, id) {
			return E('div', { 'class': 'iptv-metric' }, [
				E('span', { 'class': 'iptv-label' }, label),
				E('strong', { 'id': id }, '-')
			]);
		};
		var detail = function(label, id) {
			return E('div', { 'class': 'iptv-detail' }, [
				E('span', { 'class': 'iptv-label' }, label),
				E('span', { 'id': id }, '-')
			]);
		};

		var statusPanel = E('div', { 'class': 'iptv-status-panel' }, [
			E('div', { 'class': 'iptv-summary' }, [
				E('div', { 'class': 'iptv-result-card', 'role': 'status' }, [
					E('span', { 'class': 'iptv-label' }, _('Refresh result')),
					E('strong', { 'class': 'iptv-result', 'id': 'iptv-refresh-result', 'data-state': 'neutral' }, '-')
				]),
				E('div', { 'class': 'iptv-service-card' }, [
					detail(_('Service status'), 'iptv-service-status'),
					detail(_('Refresh state'), 'iptv-refresh-state'),
					E('div', { 'class': 'iptv-detail' }, [
						E('span', { 'class': 'iptv-label' }, _('Backend version')),
						E('span', { 'id': 'iptv-version' }, version || '-')
					])
				])
			]),
			E('div', { 'class': 'iptv-metrics' }, [
				metric(_('Channels'), 'iptv-channels'),
				metric(_('Timeshift channels'), 'iptv-timeshift'),
				metric(_('EPG matches'), 'iptv-epg-mapped'),
				metric(_('Logo matches'), 'iptv-logos')
			]),
			E('div', { 'class': 'iptv-details' }, [
				detail(_('Started at'), 'iptv-started-at'),
				detail(_('Finished at'), 'iptv-finished-at'),
				detail(_('Playlist path'), 'iptv-output')
			]),
			E('div', { 'class': 'iptv-error-panel', 'id': 'iptv-error-panel', 'role': 'alert', 'hidden': 'hidden' }, [
				E('strong', {}, _('Last error')),
				E('pre', { 'id': 'iptv-last-error' }, '-')
			])
		]);

		var styles = E('style', {}, [
			'.iptv-status-panel{margin-top:.75rem}',
			'.iptv-summary{display:grid;grid-template-columns:minmax(12rem,1fr) minmax(18rem,2fr);gap:.75rem}',
			'.iptv-result-card,.iptv-service-card,.iptv-metric,.iptv-detail{border:1px solid rgba(127,127,127,.28);border-radius:.55rem;background:rgba(127,127,127,.07)}',
			'.iptv-result-card{display:flex;flex-direction:column;justify-content:center;align-items:flex-start;padding:1rem 1.2rem}',
			'.iptv-result{display:inline-block;margin-top:.4rem;padding:.25rem .75rem;border:1px solid currentColor;border-radius:999px;font-size:1.4rem;line-height:1.3}',
			'.iptv-result[data-state="success"],[data-state="success"]{color:#2e9b50}',
			'.iptv-result[data-state="error"],[data-state="error"]{color:#d64b4b}',
			'.iptv-result[data-state="working"]{color:#368bd6}',
			'.iptv-result[data-state="neutral"]{color:inherit;opacity:.72}',
			'.iptv-service-card{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:0;background:transparent;overflow:hidden}',
			'.iptv-service-card .iptv-detail{border:0;border-radius:0;background:rgba(127,127,127,.07)}',
			'.iptv-service-card .iptv-detail+.iptv-detail{border-left:1px solid rgba(127,127,127,.22)}',
			'.iptv-label{display:block;margin-bottom:.3rem;font-size:.82rem;line-height:1.3;opacity:.72}',
			'.iptv-metrics{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:.75rem;margin-top:.75rem}',
			'.iptv-metric{padding:.8rem 1rem;min-width:0}',
			'.iptv-metric strong{font-size:1.65rem;line-height:1.2}',
			'.iptv-details{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:.75rem;margin-top:.75rem}',
			'.iptv-detail{padding:.75rem 1rem;min-width:0;overflow-wrap:anywhere}',
			'.iptv-details .iptv-detail:last-child{grid-column:1/-1}',
			'.iptv-error-panel{margin-top:.75rem;padding:.8rem 1rem;border:1px solid rgba(214,75,75,.65);border-radius:.55rem;background:rgba(214,75,75,.08)}',
			'.iptv-error-panel[hidden]{display:none}',
			'.iptv-error-panel pre{max-height:8em;margin:.5rem 0 0;padding:.65rem;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-word;background:rgba(0,0,0,.08);color:inherit}',
			'.iptv-actions{display:flex;flex-wrap:wrap;gap:.5rem}',
			'.iptv-action-hint{flex-basis:100%;margin:.25rem 0 0;opacity:.75}',
			'.iptv-actions [hidden]{display:none}',
			'.iptv-log-head{display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:.75rem 1rem}',
			'.iptv-log-actions,.iptv-log-size{display:flex;align-items:center;gap:.5rem}',
			'.iptv-log-size span{font-size:.85rem;opacity:.78}',
			'.iptv-log-size input{width:6.5rem}',
			'#iptv-log-output{max-height:24em;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;margin-top:.75rem}',
			'@media(max-width:800px){.iptv-summary{grid-template-columns:1fr}.iptv-metrics{grid-template-columns:repeat(2,minmax(0,1fr))}}',
			'@media(max-width:520px){.iptv-service-card,.iptv-details{grid-template-columns:1fr}.iptv-service-card .iptv-detail+.iptv-detail{border-left:0;border-top:1px solid rgba(127,127,127,.22)}.iptv-details .iptv-detail:last-child{grid-column:auto}}'
		].join(''));

		var viewNode = E('div', { 'class': 'cbi-map' }, [
			styles,
			E('h2', {}, _('IPTV Refresh')),
			E('div', { 'class': 'cbi-map-descr' }, _('Manage the local IPTV playlist refresh service. The API token remains on the router and is never sent to this browser.')),
			E('div', { 'class': 'cbi-section' }, [ E('h3', {}, _('Status')), statusPanel ]),
			E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Actions')),
				E('div', { 'class': 'cbi-section-node iptv-actions' }, [
					E('button', { 'class': 'btn cbi-button cbi-button-action important', 'id': 'iptv-refresh-button', 'hidden': 'hidden', 'click': ui.createHandlerFn(this, 'handleRefresh', 'refresh') }, _('Refresh using saved credentials')),
					E('button', { 'class': 'btn cbi-button cbi-button-action', 'id': 'iptv-capture-refresh-button', 'hidden': 'hidden', 'click': ui.createHandlerFn(this, 'handleRefresh', 'capture-refresh') }, _('Capture credentials and refresh')),
					E('button', { 'class': 'btn cbi-button cbi-button-positive', 'id': 'iptv-start-button', 'hidden': 'hidden', 'click': ui.createHandlerFn(this, 'handleServiceAction', 'start') }, _('Start')),
					E('button', { 'class': 'btn cbi-button cbi-button-action', 'id': 'iptv-restart-button', 'hidden': 'hidden', 'click': ui.createHandlerFn(this, 'handleServiceAction', 'restart') }, _('Restart')),
					E('button', { 'class': 'btn cbi-button cbi-button-negative', 'id': 'iptv-stop-button', 'hidden': 'hidden', 'click': ui.createHandlerFn(this, 'handleServiceAction', 'stop') }, _('Stop')),
					E('button', { 'class': 'btn cbi-button', 'id': 'iptv-download-button', 'hidden': 'hidden', 'click': ui.createHandlerFn(this, 'handleDownload') }, _('Download playlist')),
					E('p', { 'class': 'iptv-action-hint' }, _('The normal refresh does not require the STB. Use credential capture only after saved credentials expire, and keep the STB powered on while capturing.'))
				])
			]),
			E('div', { 'class': 'cbi-section' }, [
				E('div', { 'class': 'iptv-log-head' }, [
					E('h3', {}, _('Recent log')),
					E('div', { 'class': 'iptv-log-actions' }, [
						E('label', { 'class': 'iptv-log-size' }, [
							E('span', {}, _('Log size limit')),
							E('input', {
								'id': 'iptv-log-size-value', 'type': 'number', 'min': '1', 'step': '1',
								'max': String(maxLogSizeValue(logSize.unit)), 'value': logSize.value,
								'change': ui.createHandlerFn(this, 'handleLogMaxSize')
							}),
							E('select', { 'id': 'iptv-log-size-unit', 'change': ui.createHandlerFn(this, 'handleLogMaxSize') }, [
								E('option', { 'value': 'K', 'selected': logSize.unit === 'K' ? 'selected' : null }, 'KB'),
								E('option', { 'value': 'M', 'selected': logSize.unit === 'M' ? 'selected' : null }, 'MB')
							])
						]),
						E('button', { 'class': 'btn cbi-button', 'click': ui.createHandlerFn(this, 'handleReloadLog') }, _('Reload log')),
						E('button', { 'class': 'btn cbi-button cbi-button-negative', 'click': ui.createHandlerFn(this, 'handleClearLog') }, _('Clear log'))
					])
				]),
				E('pre', { 'id': 'iptv-log-output' }, log || _('No matching log entries.'))
			])
		]);

		window.setTimeout(L.bind(function() { this.updateStatus(initialStatus); }, this), 0);
		return viewNode;
	},

	handleSaveApply: null,
	handleSave: null,
	handleReset: null
});
