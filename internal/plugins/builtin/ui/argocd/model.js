// The Argo CD model every page of the built-in plugin draws from: the
// Applications and what their status says about them, the projects they are
// in, what has been deployed lately, and how Argo CD's own components are
// doing. Read once per poll through the k8sdockside bridge and worked out
// here, so the pages only have to draw.
(function () {
    'use strict';

    var KINDS = {
        apps: 'crd:applications.argoproj.io',
        projects: 'crd:appprojects.argoproj.io',
        appsets: 'crd:applicationsets.argoproj.io',
        pods: 'pods',
        events: 'events',
    };

    // Worst first, everywhere: one Degraded application among forty Healthy
    // ones is the reason anybody opened the page.
    var HEALTH_RANK = { Degraded: 0, Missing: 1, Progressing: 2, Suspended: 3, Unknown: 4, '': 4, Healthy: 5 };
    var SYNC_RANK = { OutOfSync: 0, Unknown: 1, '': 1, Synced: 2 };

    var HEALTH_TONE = { Healthy: 'ok', Progressing: 'info', Degraded: 'error', Suspended: 'paused', Missing: 'warn', Unknown: 'muted', '': 'muted' };
    var SYNC_TONE = { Synced: 'ok', OutOfSync: 'warn', Unknown: 'muted', '': 'muted' };

    // An Argo resource names its Kind ("Deployment"); the app names kinds by
    // their plural. Only the ones this plugin may read -- anything else is
    // drawn without a way in.
    var APP_KIND = {
        Deployment: 'deployments',
        StatefulSet: 'statefulsets',
        DaemonSet: 'daemonsets',
        ReplicaSet: 'replicasets',
        Pod: 'pods',
        Service: 'services',
        Ingress: 'ingresses',
        ConfigMap: 'configmaps',
        Job: 'jobs',
        CronJob: 'cronjobs',
        PersistentVolumeClaim: 'persistentvolumeclaims',
        HorizontalPodAutoscaler: 'horizontalpodautoscalers',
        NetworkPolicy: 'networkpolicies',
        ServiceAccount: 'serviceaccounts',
        Application: KINDS.apps,
        ApplicationSet: KINDS.appsets,
        AppProject: KINDS.projects,
    };

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

    function conditionTrue(obj, type) {
        var list = dig(obj, 'status.conditions') || [];
        for (var i = 0; i < list.length; i++) {
            if (list[i].type === type) return list[i].status === 'True';
        }
        return false;
    }

    // A revision written for a person: seven characters of a Git SHA, a Helm
    // chart version as it is.
    function short(rev) {
        rev = String(rev || '');
        return /^[0-9a-f]{40}$/.test(rev) ? rev.slice(0, 7) : rev;
    }

    // "https://github.com/acme/deploy.git" -> "acme/deploy".
    function repoLabel(url) {
        url = String(url || '');
        var m = /[:/]([^/:]+\/[^/]+?)(\.git)?\/?$/.exec(url);
        return m ? m[1] : url;
    }

    function time(ts) {
        var t = ts ? new Date(ts).getTime() : 0;
        return isFinite(t) ? t : 0;
    }

    function sourceOf(spec) {
        return spec.source || (spec.sources && spec.sources[0]) || {};
    }

    function buildApp(obj) {
        var spec = obj.spec || {};
        var status = obj.status || {};
        var src = sourceOf(spec);
        var dest = spec.destination || {};
        var op = status.operationState || null;
        var sync = status.sync || {};
        var resources = status.resources || [];
        var auto = !!dig(spec, 'syncPolicy.automated');

        var resourceHealth = {};
        var outOfSync = 0;
        resources.forEach(function (r) {
            var h = (r.health && r.health.status) || '';
            resourceHealth[h] = (resourceHealth[h] || 0) + 1;
            if (r.status === 'OutOfSync') outOfSync++;
        });

        var history = (status.history || [])
            .map(function (h) {
                var s = h.source || (h.sources && h.sources[0]) || {};
                return {
                    id: h.id,
                    revision: h.revision || (h.revisions && h.revisions[0]) || '',
                    deployedAt: h.deployedAt || '',
                    startedAt: h.deployStartedAt || '',
                    repo: s.repoURL || '',
                    path: s.path || s.chart || '',
                    target: s.targetRevision || '',
                    initiatedBy: dig(h, 'initiatedBy.username') || (dig(h, 'initiatedBy.automated') ? 'automated' : ''),
                };
            })
            .sort(function (a, b) {
                return time(b.deployedAt) - time(a.deployedAt);
            });

        return {
            key: obj.metadata.namespace + '/' + obj.metadata.name,
            name: obj.metadata.name,
            namespace: obj.metadata.namespace || '',
            obj: obj,
            project: spec.project || 'default',
            health: dig(status, 'health.status') || '',
            healthMessage: dig(status, 'health.message') || '',
            sync: sync.status || '',
            revision: sync.revision || (sync.revisions && sync.revisions[0]) || '',
            repo: src.repoURL || '',
            path: src.path || '',
            chart: src.chart || '',
            target: src.targetRevision || 'HEAD',
            sources: (spec.sources || []).length || 1,
            destServer: dest.server || '',
            destName: dest.name || '',
            destNamespace: dest.namespace || '',
            auto: auto,
            prune: !!dig(spec, 'syncPolicy.automated.prune'),
            selfHeal: !!dig(spec, 'syncPolicy.automated.selfHeal'),
            op: op
                ? {
                      phase: op.phase || '',
                      message: op.message || '',
                      startedAt: op.startedAt || '',
                      finishedAt: op.finishedAt || '',
                      revision: dig(op, 'syncResult.revision') || dig(op, 'operation.sync.revision') || '',
                      initiatedBy: dig(op, 'operation.initiatedBy.username') || (dig(op, 'operation.initiatedBy.automated') ? 'automated' : ''),
                      results: dig(op, 'syncResult.resources') || [],
                  }
                : null,
            history: history,
            resources: resources,
            resourceHealth: resourceHealth,
            outOfSync: outOfSync,
            conditions: status.conditions || [],
            urls: dig(status, 'summary.externalURLs') || [],
            images: dig(status, 'summary.images') || [],
            reconciledAt: status.reconciledAt || '',
            refreshing: !!(obj.metadata.annotations && obj.metadata.annotations['argocd.argoproj.io/refresh']),
        };
    }

    // Where an app deploys to, written for a person.
    function destination(app) {
        var cluster = app.destName || (app.destServer === 'https://kubernetes.default.svc' ? 'in-cluster' : app.destServer.replace(/^https?:\/\//, ''));
        return (cluster || 'in-cluster') + (app.destNamespace ? ' / ' + app.destNamespace : '');
    }

    function source(app) {
        var where = repoLabel(app.repo);
        if (app.chart) return where + ' · chart ' + app.chart + (app.target ? ' ' + app.target : '');
        return where + (app.path ? ' / ' + app.path : '') + (app.target ? ' @ ' + app.target : '');
    }

    function worstFirst(a, b) {
        var h = (HEALTH_RANK[a.health] !== undefined ? HEALTH_RANK[a.health] : 4) - (HEALTH_RANK[b.health] !== undefined ? HEALTH_RANK[b.health] : 4);
        if (h) return h;
        var s = (SYNC_RANK[a.sync] !== undefined ? SYNC_RANK[a.sync] : 1) - (SYNC_RANK[b.sync] !== undefined ? SYNC_RANK[b.sync] : 1);
        if (s) return s;
        return a.name.localeCompare(b.name);
    }

    function opFailed(app) {
        return !!app.op && (app.op.phase === 'Failed' || app.op.phase === 'Error');
    }

    function opRunning(app) {
        return !!app.op && (app.op.phase === 'Running' || app.op.phase === 'Terminating');
    }

    // What wants a person, worst first, with what it is about and what to do.
    function attention(apps) {
        var out = [];
        apps.forEach(function (app) {
            if (app.health === 'Degraded') {
                out.push({ tone: 'error', app: app, what: 'Degraded', text: app.healthMessage || degradedResources(app) || 'Something it deploys is not working.', fix: 'refresh' });
            }
            if (opFailed(app)) {
                out.push({ tone: 'error', app: app, what: 'Sync failed', text: app.op.message || 'The last sync did not complete.', fix: 'sync' });
            }
            app.conditions.forEach(function (c) {
                if (/Error$/.test(c.type)) out.push({ tone: 'error', app: app, what: c.type.replace(/([a-z])([A-Z])/g, '$1 $2'), text: c.message || '', fix: 'refresh' });
            });
            if (app.health === 'Missing') {
                out.push({ tone: 'warn', app: app, what: 'Missing', text: 'What Git describes has not been created in the cluster.', fix: 'sync' });
            }
            // Drift is only worth its own line when nothing worse about the
            // same application already says so: Missing and a failed sync are
            // both out of sync by definition.
            var worse = app.health === 'Missing' || opFailed(app);
            if (app.sync === 'OutOfSync' && !opRunning(app) && !worse) {
                var n = app.outOfSync;
                out.push({
                    tone: 'warn',
                    app: app,
                    what: 'Out of sync',
                    text: (n ? n + (n === 1 ? ' resource differs' : ' resources differ') + ' from Git' : 'The cluster differs from Git') + (app.auto ? ', though it syncs itself.' : ', and it only syncs when asked.'),
                    fix: 'sync',
                });
            }
        });
        var rank = { error: 0, warn: 1 };
        return out.sort(function (a, b) {
            return rank[a.tone] - rank[b.tone] || a.app.name.localeCompare(b.app.name);
        });
    }

    function degradedResources(app) {
        var bad = app.resources.filter(function (r) {
            return r.health && r.health.status === 'Degraded';
        });
        if (!bad.length) return '';
        var first = bad[0];
        return first.kind + ' ' + first.name + (first.health.message ? ': ' + first.health.message : ' is degraded') + (bad.length > 1 ? ' (and ' + (bad.length - 1) + ' more)' : '');
    }

    // Every deployment Argo CD remembers, newest first, and anything syncing
    // right now above them.
    function stream(apps, limit) {
        var out = [];
        apps.forEach(function (app) {
            if (opRunning(app)) {
                out.push({ app: app, running: true, when: app.op.startedAt, revision: app.op.revision, by: app.op.initiatedBy, message: app.op.message });
            }
            app.history.slice(0, 5).forEach(function (h) {
                out.push({ app: app, running: false, when: h.deployedAt, revision: h.revision, by: h.initiatedBy, repo: h.repo, path: h.path });
            });
        });
        return out
            .sort(function (a, b) {
                if (a.running !== b.running) return a.running ? -1 : 1;
                return time(b.when) - time(a.when);
            })
            .slice(0, limit || 14);
    }

    // Argo CD's own pods by component, from the labels its manifests and
    // Helm chart both set.
    function components(pods) {
        var by = {};
        pods.forEach(function (pod) {
            var l = labelsOf(pod);
            var name = (l['app.kubernetes.io/name'] || l['app.kubernetes.io/component'] || pod.metadata.name).replace(/^argocd-/, '');
            var c = (by[name] = by[name] || { name: name, ready: 0, total: 0, restarts: 0, pods: [] });
            c.total++;
            if (dig(pod, 'status.phase') === 'Running' && conditionTrue(pod, 'Ready')) c.ready++;
            (dig(pod, 'status.containerStatuses') || []).forEach(function (cs) {
                c.restarts += cs.restartCount || 0;
            });
            c.pods.push(pod);
        });
        var order = ['server', 'repo-server', 'application-controller', 'applicationset-controller', 'redis', 'dex-server', 'notifications-controller'];
        return Object.keys(by)
            .map(function (k) {
                return by[k];
            })
            .sort(function (a, b) {
                var ia = order.indexOf(a.name);
                var ib = order.indexOf(b.name);
                return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib) || a.name.localeCompare(b.name);
            });
    }

    // "quay.io/argoproj/argocd:v2.13.1" -> "v2.13.1".
    function versionOf(pods) {
        for (var i = 0; i < pods.length; i++) {
            var containers = dig(pods[i], 'spec.containers') || [];
            for (var j = 0; j < containers.length; j++) {
                var image = containers[j].image || '';
                var at = image.lastIndexOf(':');
                if (/argoproj\/argocd/.test(image) && at > image.lastIndexOf('/')) return image.slice(at + 1).replace(/@.*$/, '');
            }
        }
        return '';
    }

    function soft(promise) {
        return promise.then(
            function (items) {
                return { ok: true, items: items || [], error: '' };
            },
            function (err) {
                return { ok: false, items: [], error: (err && err.message) || String(err) };
            },
        );
    }

    function version(list) {
        return list
            .map(function (o) {
                return o.metadata.uid + '@' + o.metadata.resourceVersion;
            })
            .join(',');
    }

    function count(list, field) {
        var out = {};
        list.forEach(function (x) {
            out[x[field]] = (out[x[field]] || 0) + 1;
        });
        return out;
    }

    function load(sdk) {
        function list(kind, selector) {
            return soft(sdk.list({ kind: kind, namespace: '', selector: selector || '' }));
        }
        return Promise.all([list(KINDS.apps), list(KINDS.projects), list(KINDS.appsets), list(KINDS.pods, 'app.kubernetes.io/part-of=argocd')]).then(function (got) {
            var apps = got[0].items.map(buildApp).sort(worstFirst);
            var pods = got[3].items;

            var projects = {};
            got[1].items.forEach(function (p) {
                projects[p.metadata.name] = { name: p.metadata.name, obj: p, description: dig(p, 'spec.description') || '', apps: [] };
            });
            apps.forEach(function (a) {
                (projects[a.project] = projects[a.project] || { name: a.project, obj: null, description: '', apps: [] }).apps.push(a);
            });

            var repos = {};
            apps.forEach(function (a) {
                if (a.repo) repos[a.repo] = (repos[a.repo] || 0) + 1;
            });

            var anchor = pods.find(function (p) {
                return /server/.test(labelsOf(p)['app.kubernetes.io/name'] || '');
            });

            return {
                installed: got[0].ok,
                missing: got[0].error,
                kinds: { projects: got[1].ok, appsets: got[2].ok },
                apps: apps,
                projects: Object.keys(projects)
                    .map(function (k) {
                        return projects[k];
                    })
                    .sort(function (a, b) {
                        return b.apps.length - a.apps.length || a.name.localeCompare(b.name);
                    }),
                appsets: got[2].items,
                repos: repos,
                health: count(apps, 'health'),
                sync: count(apps, 'sync'),
                auto: apps.filter(function (a) {
                    return a.auto;
                }).length,
                attention: attention(apps),
                components: components(pods),
                componentsKnown: got[3].ok && pods.length > 0,
                version: versionOf(pods),
                namespace: anchor ? anchor.metadata.namespace : pods[0] ? pods[0].metadata.namespace : '',
                sig: [got[0].items, got[1].items, got[2].items, pods].map(version).join('|'),
            };
        });
    }

    // The two patches Argo CD itself makes for its Sync and Refresh buttons.
    var patches = {
        // The operation `argocd app sync` writes: the controller notices it on
        // the Application and runs the sync.
        sync: function () {
            return { operation: { initiatedBy: { username: 'k8sdockside' }, sync: { syncStrategy: { hook: {} } } } };
        },
        // Argo CD's own refresh annotation; the controller removes it again.
        refresh: function (hard) {
            return { metadata: { annotations: { 'argocd.argoproj.io/refresh': hard ? 'hard' : 'normal' } } };
        },
    };

    function appRef(app) {
        return { kind: KINDS.apps, namespace: app.namespace, name: app.name };
    }

    window.Argo = {
        KINDS: KINDS,
        APP_KIND: APP_KIND,
        HEALTH_TONE: HEALTH_TONE,
        SYNC_TONE: SYNC_TONE,
        HEALTH_RANK: HEALTH_RANK,
        load: load,
        buildApp: buildApp,
        attention: attention,
        stream: stream,
        short: short,
        repoLabel: repoLabel,
        destination: destination,
        source: source,
        opRunning: opRunning,
        opFailed: opFailed,
        worstFirst: worstFirst,
        patches: patches,
        appRef: appRef,
        dig: dig,
        time: time,
    };
})();
