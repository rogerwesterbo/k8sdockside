// The Prometheus model the built-in plugin's pages draw from: the servers and
// Alertmanagers the operator runs, every alerting rule in every
// PrometheusRule, whether a Prometheus actually loads each of those objects,
// and -- from the plugin's overview charts -- what is firing and what fired.
// Read through the k8sdockside bridge and worked out here, so the pages only
// have to draw.
(function () {
    'use strict';

    var KINDS = {
        servers: 'crd:prometheuses.monitoring.coreos.com',
        alertmanagers: 'crd:alertmanagers.monitoring.coreos.com',
        rules: 'crd:prometheusrules.monitoring.coreos.com',
        namespaces: 'namespaces',
    };

    // Worst first, everywhere. Severity is only a label, so the usual
    // spellings are all read.
    var SEVERITY_RANK = { critical: 0, error: 0, high: 0, page: 0, warning: 1, warn: 1, medium: 1, info: 2, low: 2, none: 3, '': 3 };
    var SEVERITY_TONE = { critical: 'error', error: 'error', high: 'error', page: 'error', warning: 'warn', warn: 'warn', medium: 'warn', info: 'info', low: 'info', none: 'muted', '': 'muted' };

    // Alerts that fire on purpose, all the time: Watchdog proves the path is
    // alive, InfoInhibitor exists to silence info alerts. Counting them as
    // "firing" would make every healthy cluster look like it has a problem.
    var ALWAYS_ON = { Watchdog: true, InfoInhibitor: true };

    function severityRank(s) {
        var r = SEVERITY_RANK[String(s || '').toLowerCase()];
        return r === undefined ? 2.5 : r;
    }

    function severityTone(s) {
        return SEVERITY_TONE[String(s || '').toLowerCase()] || 'muted';
    }

    function dig(obj, path) {
        var at = obj;
        var keys = path.split('.');
        for (var i = 0; i < keys.length; i++) {
            if (at === null || at === undefined) return undefined;
            at = at[keys[i]];
        }
        return at;
    }

    function labelsOf(obj) {
        return (obj && obj.metadata && obj.metadata.labels) || {};
    }

    function keyOf(obj) {
        return obj.metadata.namespace + '/' + obj.metadata.name;
    }

    // Installed by Helm, and so rewritten by the next helm upgrade.
    function helm(obj) {
        var l = labelsOf(obj);
        return l['app.kubernetes.io/managed-by'] === 'Helm' || l.heritage === 'Helm';
    }

    // Whether a Prometheus or Alertmanager is serving: its Available
    // condition, or failing that its replica counts. true, false,
    // 'degraded', or null when it does not say.
    function available(obj) {
        var conds = dig(obj, 'status.conditions') || [];
        for (var i = 0; i < conds.length; i++) {
            if (conds[i].type !== 'Available') continue;
            if (conds[i].status === 'True') return true;
            return conds[i].status === 'Degraded' ? 'degraded' : false;
        }
        var st = obj.status || {};
        if (st.availableReplicas === undefined) return null;
        return st.availableReplicas > 0;
    }

    // ----- which Prometheus loads which rules ---------------------------------

    // A Kubernetes label selector, applied: matchLabels and matchExpressions.
    function matchSelector(selector, labels) {
        labels = labels || {};
        var ml = selector.matchLabels || {};
        for (var k in ml) {
            if (labels[k] !== ml[k]) return false;
        }
        var ex = selector.matchExpressions || [];
        for (var i = 0; i < ex.length; i++) {
            var e = ex[i];
            var has = Object.prototype.hasOwnProperty.call(labels, e.key);
            var values = e.values || [];
            switch (e.operator) {
                case 'In':
                    if (!has || values.indexOf(labels[e.key]) < 0) return false;
                    break;
                case 'NotIn':
                    if (has && values.indexOf(labels[e.key]) >= 0) return false;
                    break;
                case 'Exists':
                    if (!has) return false;
                    break;
                case 'DoesNotExist':
                    if (has) return false;
                    break;
                default:
                    return false;
            }
        }
        return true;
    }

    function describeSelector(sel) {
        var parts = [];
        Object.keys((sel && sel.matchLabels) || {}).forEach(function (k) {
            parts.push(k + '=' + sel.matchLabels[k]);
        });
        ((sel && sel.matchExpressions) || []).forEach(function (e) {
            parts.push(e.key + ' ' + e.operator + (e.values && e.values.length ? ' (' + e.values.join(', ') + ')' : ''));
        });
        return parts.length ? parts.join(', ') : 'anything';
    }

    // Why a Prometheus would not load a PrometheusRule with these labels in
    // this namespace -- or '' when it would. The operator's rules: a null
    // ruleSelector loads nothing and an empty one everything; a null
    // ruleNamespaceSelector means the Prometheus's own namespace only.
    function whyNot(server, labels, namespace, nsLabels) {
        var spec = server.spec || {};
        if (!spec.ruleSelector) return server.metadata.name + ' has no ruleSelector, so it loads no PrometheusRules at all';
        if (!matchSelector(spec.ruleSelector, labels)) return server.metadata.name + ' only loads rules labelled ' + describeSelector(spec.ruleSelector);
        if (!spec.ruleNamespaceSelector) {
            if (namespace !== server.metadata.namespace) return server.metadata.name + ' only loads rules from its own namespace, ' + server.metadata.namespace;
        } else if (!matchSelector(spec.ruleNamespaceSelector, (nsLabels || {})[namespace] || {})) {
            return server.metadata.name + ' only loads rules from namespaces labelled ' + describeSelector(spec.ruleNamespaceSelector);
        }
        return '';
    }

    function loads(server, labels, namespace, nsLabels) {
        return whyNot(server, labels, namespace, nsLabels) === '';
    }

    // The servers that load one rule object.
    function loadedBy(model, obj) {
        return model.servers.filter(function (s) {
            return loads(s, labelsOf(obj), obj.metadata.namespace, model.nsLabels);
        });
    }

    // ----- the rules ------------------------------------------------------------

    // Every alerting rule, flattened out of its object and group, remembering
    // where it came from so it can be written back.
    function flatten(objs) {
        var alerts = [];
        var recording = 0;
        objs.forEach(function (obj) {
            (dig(obj, 'spec.groups') || []).forEach(function (g, gi) {
                (g.rules || []).forEach(function (r, ri) {
                    if (!r.alert) {
                        if (r.record) recording++;
                        return;
                    }
                    var labels = r.labels || {};
                    alerts.push({
                        key: keyOf(obj) + '/' + gi + '/' + ri,
                        obj: obj,
                        namespace: obj.metadata.namespace,
                        objName: obj.metadata.name,
                        group: g.name || '',
                        groupIndex: gi,
                        ruleIndex: ri,
                        alert: r.alert,
                        expr: r.expr === undefined || r.expr === null ? '' : String(r.expr),
                        for: r['for'] || '',
                        labels: labels,
                        annotations: r.annotations || {},
                        severity: labels.severity || '',
                        raw: r,
                    });
                });
            });
        });
        return { alerts: alerts, recording: recording };
    }

    // Annotations are Go templates, filled in per alert by Prometheus. Out of
    // that context the placeholders are noise, so they are shortened.
    function untemplate(text) {
        return String(text || '')
            .replace(/\{\{[^}]*\}\}/g, '…')
            .replace(/\s+/g, ' ')
            .trim();
    }

    // ----- reading the cluster --------------------------------------------------

    function load(sdk) {
        var errors = {};
        function list(key, kind) {
            return sdk.list({ kind: kind }).catch(function (err) {
                errors[key] = (err && err.message) || String(err);
                return [];
            });
        }
        return Promise.all([list('servers', KINDS.servers), list('alertmanagers', KINDS.alertmanagers), list('rules', KINDS.rules), list('namespaces', KINDS.namespaces)]).then(function (r) {
            var nsLabels = {};
            r[3].forEach(function (ns) {
                nsLabels[ns.metadata.name] = labelsOf(ns);
            });
            var flat = flatten(r[2]);
            var sig = r
                .map(function (items) {
                    return items
                        .map(function (o) {
                            return o.metadata.uid + ':' + o.metadata.resourceVersion;
                        })
                        .join(',');
                })
                .join('|');
            return {
                installed: !errors.servers,
                servers: r[0],
                alertmanagers: r[1],
                ruleObjs: r[2].slice().sort(function (a, b) {
                    return keyOf(a).localeCompare(keyOf(b));
                }),
                alerts: flat.alerts,
                recording: flat.recording,
                nsLabels: nsLabels,
                namespaces: Object.keys(nsLabels).sort(),
                errors: errors,
                sig: sig + JSON.stringify(errors),
            };
        });
    }

    // ----- the charts -----------------------------------------------------------

    function chart(panel, id) {
        var list = (panel && panel.charts) || [];
        for (var i = 0; i < list.length; i++) if (list[i].id === id) return list[i];
        return null;
    }

    // The resolution the app asked Prometheus for: 120 steps over the range.
    function stepOf(panel) {
        return Math.max(15, (((panel && panel.range) || 60) * 60) / 120);
    }

    // The newest sample anywhere, which is "now" as far as the charts know.
    function nowOf(panel) {
        var t = 0;
        ((panel && panel.charts) || []).forEach(function (c) {
            c.series.forEach(function (s) {
                var p = s.points[s.points.length - 1];
                if (p && p.t > t) t = p.t;
            });
        });
        return t || Date.now() / 1000;
    }

    // A series' last sample, if it is current -- a job that vanished an hour
    // ago is not "up" now just because its last sample said so.
    function fresh(panel, series) {
        var p = series.points[series.points.length - 1];
        return p && nowOf(panel) - p.t <= stepOf(panel) * 2 ? p : null;
    }

    function latestBy(panel, id) {
        var out = {};
        var c = chart(panel, id);
        if (!c) return out;
        c.series.forEach(function (s) {
            var p = fresh(panel, s);
            if (p) out[s.name] = (out[s.name] || 0) + p.v;
        });
        return out;
    }

    function latestSum(panel, id) {
        var c = chart(panel, id);
        if (!c || c.error) return null;
        var any = false;
        var sum = 0;
        c.series.forEach(function (s) {
            var p = fresh(panel, s);
            if (p) {
                any = true;
                sum += p.v;
            }
        });
        return any ? sum : null;
    }

    function latestMax(panel, id) {
        var c = chart(panel, id);
        if (!c || c.error) return null;
        var max = null;
        c.series.forEach(function (s) {
            var p = fresh(panel, s);
            if (p && (max === null || p.v > max)) max = p.v;
        });
        return max;
    }

    // Every series of a chart added together at each moment, for a sparkline.
    // keep(name) chooses the series; all of them when it is omitted.
    function sumOverTime(panel, id, keep, max) {
        var c = chart(panel, id);
        if (!c) return [];
        var at = {};
        c.series.forEach(function (s) {
            if (keep && !keep(s.name)) return;
            s.points.forEach(function (p) {
                if (!isFinite(p.v)) return;
                at[p.t] = at[p.t] === undefined ? p.v : max ? Math.max(at[p.t], p.v) : at[p.t] + p.v;
            });
        });
        return Object.keys(at)
            .map(Number)
            .sort(function (a, b) {
                return a - b;
            })
            .map(function (t) {
                return { t: t, v: at[t] };
            });
    }

    // A series with no legend is named by its whole label set:
    // "alertname=X, alertstate=firing" -> { alertname: 'X', alertstate: 'firing' }.
    function parseLabels(name) {
        var out = {};
        String(name || '')
            .split(', ')
            .forEach(function (part) {
                var i = part.indexOf('=');
                if (i > 0) out[part.slice(0, i)] = part.slice(i + 1);
            });
        return out;
    }

    // Contiguous stretches of samples: the times an alert was active.
    function runs(points, step) {
        var out = [];
        var cur = null;
        points.forEach(function (p) {
            if (!isFinite(p.v)) return;
            if (cur && p.t - cur.end <= step * 1.5) {
                cur.end = p.t;
            } else {
                cur = { start: p.t, end: p.t };
                out.push(cur);
            }
        });
        return out;
    }

    function byStart(a, b) {
        return a.start - b.start;
    }

    // What has been pending and firing over the charts' window, one lane per
    // alert and namespace, with what each is doing now. Worst and most recent
    // first.
    function activeAlerts(panel) {
        var c = chart(panel, 'alerts-active');
        if (!c) return { known: false, lanes: [], now: nowOf(panel), step: stepOf(panel) };
        var step = stepOf(panel);
        var now = nowOf(panel);
        var byKey = {};
        c.series.forEach(function (s) {
            var l = parseLabels(s.name);
            if (!l.alertname) return;
            var key = l.alertname + '|' + (l.namespace || '');
            var lane = byKey[key];
            if (!lane) {
                lane = byKey[key] = { key: key, alertname: l.alertname, namespace: l.namespace || '', severity: l.severity || '', firing: [], pending: [], state: '', since: 0, last: 0 };
            }
            if (!lane.severity && l.severity) lane.severity = l.severity;
            var r = runs(s.points, step);
            if (l.alertstate === 'firing') lane.firing = lane.firing.concat(r);
            else lane.pending = lane.pending.concat(r);
        });
        var lanes = Object.keys(byKey).map(function (k) {
            var lane = byKey[k];
            lane.firing.sort(byStart);
            lane.pending.sort(byStart);
            var f = lane.firing[lane.firing.length - 1];
            var p = lane.pending[lane.pending.length - 1];
            if (f && now - f.end <= step * 1.5) {
                lane.state = 'firing';
                lane.since = f.start;
            } else if (p && now - p.end <= step * 1.5) {
                lane.state = 'pending';
                lane.since = p.start;
            }
            lane.last = Math.max(f ? f.end : 0, p ? p.end : 0);
            return lane;
        });
        var STATE_RANK = { firing: 0, pending: 1, '': 2 };
        lanes.sort(function (a, b) {
            return STATE_RANK[a.state] - STATE_RANK[b.state] || severityRank(a.severity) - severityRank(b.severity) || b.last - a.last || a.alertname.localeCompare(b.alertname);
        });
        return { known: true, error: c.error, lanes: lanes, now: now, step: step, range: panel.range };
    }

    // ----- writing numbers for a person -----------------------------------------

    function compact(v) {
        var a = Math.abs(v);
        if (a >= 1e9) return (v / 1e9).toFixed(a >= 1e10 ? 0 : 1) + 'G';
        if (a >= 1e6) return (v / 1e6).toFixed(a >= 1e7 ? 0 : 1) + 'M';
        if (a >= 1e4) return Math.round(v / 1e3) + 'k';
        if (a >= 1e3) return (v / 1e3).toFixed(1) + 'k';
        if (a >= 10 || v === Math.round(v)) return String(Math.round(v));
        return v.toFixed(a >= 1 ? 1 : 2);
    }

    function bytes(v) {
        var units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];
        var i = 0;
        while (Math.abs(v) >= 1024 && i < units.length - 1) {
            v /= 1024;
            i++;
        }
        return (i === 0 ? Math.round(v) : v.toFixed(Math.abs(v) >= 100 ? 0 : 1)) + ' ' + units[i];
    }

    function fmt(v, unit) {
        if (v === null || v === undefined || !isFinite(v)) return '—';
        switch (unit) {
            case 'bytes':
                return bytes(v);
            case 'bytes/s':
                return bytes(v) + '/s';
            case 'percent':
                return (v * 100).toFixed(v < 0.1 ? 1 : 0) + '%';
            case 'ops/s':
                return compact(v) + '/s';
            case 'seconds':
                return v < 1 ? Math.round(v * 1000) + ' ms' : v.toFixed(1) + ' s';
            case 'cores':
                return v.toFixed(2);
            default:
                return compact(v);
        }
    }

    function duration(seconds) {
        seconds = Math.max(0, Math.round(seconds));
        if (seconds < 60) return seconds + 's';
        var m = Math.floor(seconds / 60);
        if (m < 60) return m + 'm';
        var h = Math.floor(m / 60);
        if (h < 48) return h + 'h' + (m % 60 ? ' ' + (m % 60) + 'm' : '');
        return Math.floor(h / 24) + 'd' + (h % 24 ? ' ' + (h % 24) + 'h' : '');
    }

    // ----- YAML, for showing a rule the way it will be read -------------------

    function yamlScalar(v) {
        if (typeof v === 'number' || typeof v === 'boolean') return String(v);
        if (v === null || v === undefined) return 'null';
        var s = String(v);
        // Quoted unless it cannot be read as anything but a string: 10m is
        // plain, 10 and 1e3 are numbers, yes and off are booleans.
        var plain = /^[A-Za-z0-9_./][A-Za-z0-9_ ./()-]*$/.test(s) && !/\s$/.test(s) && !/^[-+]?(\d[\d_]*)?\.?\d*([eE][-+]?\d+)?$/.test(s) && !/^(true|false|null|yes|no|on|off|y|n|~)$/i.test(s);
        return plain ? s : JSON.stringify(s);
    }

    function toYaml(v, indent) {
        if (Array.isArray(v)) {
            if (!v.length) return ' []';
            return v
                .map(function (item) {
                    var body = toYaml(item, indent + '  ');
                    if (item && typeof item === 'object' && !Array.isArray(item) && Object.keys(item).length) {
                        return '\n' + indent + '- ' + body.replace(/^\n\s*/, '');
                    }
                    return '\n' + indent + '-' + body;
                })
                .join('');
        }
        if (v && typeof v === 'object') {
            var keys = Object.keys(v);
            if (!keys.length) return ' {}';
            return keys
                .map(function (k) {
                    return '\n' + indent + yamlScalar(k) + ':' + toYaml(v[k], indent + '  ');
                })
                .join('');
        }
        if (typeof v === 'string' && v.indexOf('\n') >= 0 && !/^\s/.test(v)) {
            return (
                ' |-' +
                v
                    .split('\n')
                    .map(function (line) {
                        return '\n' + (line ? indent + line : '');
                    })
                    .join('')
            );
        }
        return ' ' + yamlScalar(v);
    }

    function yaml(v) {
        return toYaml(v, '').replace(/^\n/, '') + '\n';
    }

    // ----- templates for a new alert -------------------------------------------

    // Starting points, not finished rules: each is the common shape of an
    // alert people end up writing, with the metric names kube-prometheus-stack
    // sets up. Every field is editable once picked.
    var TEMPLATES = [
        { id: 'blank', label: 'Blank', alert: '', expr: '', for: '5m', severity: 'warning', summary: '', description: '' },
        {
            id: 'target-down',
            label: 'A scrape target is down',
            alert: 'TargetDown',
            expr: 'up == 0',
            for: '5m',
            severity: 'warning',
            summary: '{{ $labels.job }} target {{ $labels.instance }} is down',
            description: 'Prometheus has not been able to scrape {{ $labels.instance }} of job {{ $labels.job }} for five minutes.',
        },
        {
            id: 'restarts',
            label: 'A pod keeps restarting',
            alert: 'PodRestartingOften',
            expr: 'increase(kube_pod_container_status_restarts_total[15m]) > 3',
            for: '5m',
            severity: 'warning',
            summary: '{{ $labels.namespace }}/{{ $labels.pod }} is restarting',
            description: 'Container {{ $labels.container }} restarted {{ $value | humanize }} times in the last 15 minutes.',
        },
        {
            id: 'memory-limit',
            label: 'A container is near its memory limit',
            alert: 'ContainerNearMemoryLimit',
            expr: 'max by (namespace, pod, container) (container_memory_working_set_bytes{container!=""})\n  / max by (namespace, pod, container) (kube_pod_container_resource_limits{resource="memory"})\n  > 0.9',
            for: '10m',
            severity: 'warning',
            summary: '{{ $labels.namespace }}/{{ $labels.pod }} is at {{ $value | humanizePercentage }} of its memory limit',
            description: 'Container {{ $labels.container }} will be OOM-killed if its working set reaches the limit.',
        },
        {
            id: 'pvc-full',
            label: 'A volume is filling up',
            alert: 'PersistentVolumeFillingUp',
            expr: 'kubelet_volume_stats_used_bytes / kubelet_volume_stats_capacity_bytes > 0.85',
            for: '15m',
            severity: 'warning',
            summary: '{{ $labels.namespace }}/{{ $labels.persistentvolumeclaim }} is {{ $value | humanizePercentage }} full',
            description: 'The volume claimed by {{ $labels.persistentvolumeclaim }} has less than 15% left.',
        },
        {
            id: 'throttling',
            label: 'CPU throttling is high',
            alert: 'CPUThrottlingHigh',
            expr: 'sum by (namespace, pod) (rate(container_cpu_cfs_throttled_periods_total{container!=""}[5m]))\n  / sum by (namespace, pod) (rate(container_cpu_cfs_periods_total{container!=""}[5m]))\n  > 0.5',
            for: '15m',
            severity: 'info',
            summary: '{{ $labels.namespace }}/{{ $labels.pod }} is throttled {{ $value | humanizePercentage }} of the time',
            description: 'Its CPU limit is below what it needs; it is being held back, not the node.',
        },
        {
            id: 'deployment-replicas',
            label: 'A Deployment is missing replicas',
            alert: 'DeploymentReplicasMissing',
            expr: 'kube_deployment_status_replicas_available < kube_deployment_spec_replicas',
            for: '15m',
            severity: 'warning',
            summary: '{{ $labels.namespace }}/{{ $labels.deployment }} has fewer replicas available than it asks for',
            description: 'Only {{ $value }} replicas are available.',
        },
        {
            id: 'node-disk',
            label: 'A node disk is filling up',
            alert: 'NodeDiskFillingUp',
            expr: '1 - node_filesystem_avail_bytes{fstype!~"tmpfs|overlay|squashfs"} / node_filesystem_size_bytes{fstype!~"tmpfs|overlay|squashfs"} > 0.85',
            for: '15m',
            severity: 'warning',
            summary: '{{ $labels.instance }} {{ $labels.mountpoint }} is {{ $value | humanizePercentage }} full',
            description: 'When a node disk fills, the kubelet starts evicting pods to get space back.',
        },
        {
            id: 'apiserver-errors',
            label: 'The API server is answering with errors',
            alert: 'APIServerErrorsHigh',
            expr: 'sum(rate(apiserver_request_total{code=~"5.."}[5m])) / sum(rate(apiserver_request_total[5m])) > 0.05',
            for: '10m',
            severity: 'critical',
            summary: 'The API server answers {{ $value | humanizePercentage }} of requests with an error',
            description: 'More than 5% of API requests have failed with a 5xx for ten minutes.',
        },
    ];

    window.Prom = {
        KINDS: KINDS,
        ALWAYS_ON: ALWAYS_ON,
        TEMPLATES: TEMPLATES,
        severityRank: severityRank,
        severityTone: severityTone,
        dig: dig,
        labelsOf: labelsOf,
        keyOf: keyOf,
        helm: helm,
        available: available,
        matchSelector: matchSelector,
        describeSelector: describeSelector,
        whyNot: whyNot,
        loads: loads,
        loadedBy: loadedBy,
        untemplate: untemplate,
        load: load,
        chart: chart,
        stepOf: stepOf,
        latestBy: latestBy,
        latestSum: latestSum,
        latestMax: latestMax,
        sumOverTime: sumOverTime,
        parseLabels: parseLabels,
        activeAlerts: activeAlerts,
        fmt: fmt,
        duration: duration,
        yaml: yaml,
    };
})();
