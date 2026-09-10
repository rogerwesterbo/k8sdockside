// The Argo CD map: every Application as a card, unfolding into the resources
// its status says it manages. Plain script, no build step -- the point is to
// show the bridge, not a framework. See ../README.md for building one with
// Svelte or anything else.
//
// Everything that comes from the cluster is written with textContent, never
// innerHTML: the frame is sandboxed, but a page that let a resource's name run
// as markup would be handing that name the bridge.
(function () {
    'use strict';

    var sdk = window.k8sdockside;
    var APPS = 'crd:applications.argoproj.io';

    var state = {
        items: [],
        filter: '',
        namespace: '',
        readable: [],
        write: false,
        error: '',
        loaded: false,
        // Which cards have their resource list unfolded, so a refresh does not
        // fold them all back up.
        unfolded: {},
    };
    var stopWatching = null;

    var $ = function (id) {
        return document.getElementById(id);
    };

    function el(tag, className, text) {
        var node = document.createElement(tag);
        if (className) node.className = className;
        if (text !== undefined) node.textContent = text;
        return node;
    }

    function healthTone(status) {
        switch (status) {
            case 'Healthy':
                return 'ok';
            case 'Progressing':
                return 'info';
            case 'Degraded':
                return 'error';
            case 'Missing':
            case 'Suspended':
                return 'warn';
            default:
                return '';
        }
    }

    function syncTone(status) {
        if (status === 'Synced') return 'ok';
        if (status === 'OutOfSync') return 'warn';
        return '';
    }

    // An Argo resource names a Kind ("Deployment"); the app names kinds by
    // their plural ("deployments"). Good enough for the built-in kinds this
    // plugin declares; anything else is shown without a link.
    function appKindFor(resource) {
        var kind = (resource.kind || '').toLowerCase();
        if (!kind) return '';
        if (kind.endsWith('y')) return kind.slice(0, -1) + 'ies';
        if (kind.endsWith('s')) return kind + 'es';
        return kind + 's';
    }

    function source(app) {
        var spec = app.spec || {};
        var src = spec.source || (spec.sources && spec.sources[0]) || {};
        var where = src.repoURL || '';
        if (src.path) where += ' · ' + src.path;
        if (src.chart) where += ' · chart ' + src.chart;
        if (src.targetRevision) where += ' @ ' + src.targetRevision;
        return where;
    }

    function fail(err) {
        state.error = err && err.message ? err.message : String(err);
        render();
    }

    function sync(app) {
        sdk.patch({
            kind: APPS,
            namespace: app.metadata.namespace,
            name: app.metadata.name,
            // The same operation `argocd app sync` writes: the controller
            // notices it on the Application and runs the sync.
            patch: {
                operation: {
                    initiatedBy: { username: 'k8sdockside' },
                    sync: { syncStrategy: { hook: {} } },
                },
            },
        }).catch(fail);
    }

    function refresh(app) {
        sdk.patch({
            kind: APPS,
            namespace: app.metadata.namespace,
            name: app.metadata.name,
            // Argo CD's own refresh annotation; the controller removes it again.
            patch: { metadata: { annotations: { 'argocd.argoproj.io/refresh': 'normal' } } },
        }).catch(fail);
    }

    function resourceRow(resource) {
        var li = el('li');
        li.appendChild(el('span', 'kind', resource.kind || ''));

        var name = el('span', 'res');
        var kind = appKindFor(resource);
        if (kind && state.readable.indexOf(kind) >= 0) {
            var link = el('a', '', resource.name);
            link.title = 'Open in the details panel';
            link.addEventListener('click', function () {
                sdk.open({ kind: kind, namespace: resource.namespace || '', name: resource.name }).catch(fail);
            });
            name.appendChild(link);
        } else {
            name.textContent = resource.name;
        }
        li.appendChild(name);

        var health = resource.health && resource.health.status;
        if (health) li.appendChild(el('span', 'pill ' + healthTone(health), health));
        if (resource.status) li.appendChild(el('span', 'pill ' + syncTone(resource.status), resource.status));
        return li;
    }

    function card(app) {
        var status = app.status || {};
        var health = (status.health && status.health.status) || 'Unknown';
        var synced = (status.sync && status.sync.status) || 'Unknown';
        var key = app.metadata.namespace + '/' + app.metadata.name;

        var article = el('article', 'app');
        var head = el('div', 'head');
        var name = el('span', 'name', app.metadata.name);
        name.title = key;
        head.appendChild(name);
        head.appendChild(el('span', 'pill ' + healthTone(health), health));
        head.appendChild(el('span', 'pill ' + syncTone(synced), synced));
        article.appendChild(head);

        article.appendChild(el('div', 'where', source(app)));

        var actions = el('div', 'actions');
        var open = el('button', '', 'Details');
        open.addEventListener('click', function () {
            sdk.open({ kind: APPS, namespace: app.metadata.namespace, name: app.metadata.name }).catch(fail);
        });
        actions.appendChild(open);
        var yaml = el('button', '', 'YAML');
        yaml.addEventListener('click', function () {
            sdk.edit({ kind: APPS, namespace: app.metadata.namespace, name: app.metadata.name }).catch(fail);
        });
        actions.appendChild(yaml);
        if (state.write) {
            var refreshButton = el('button', '', 'Refresh');
            refreshButton.addEventListener('click', function () {
                refresh(app);
            });
            actions.appendChild(refreshButton);
            var syncButton = el('button', 'primary', 'Sync');
            syncButton.addEventListener('click', function () {
                sync(app);
            });
            actions.appendChild(syncButton);
        }
        article.appendChild(actions);

        var resources = status.resources || [];
        if (resources.length > 0) {
            var details = el('details');
            details.open = !!state.unfolded[key];
            details.addEventListener('toggle', function () {
                state.unfolded[key] = details.open;
            });
            details.appendChild(el('summary', '', resources.length + ' resource' + (resources.length === 1 ? '' : 's')));
            var list = el('ul');
            resources.forEach(function (r) {
                list.appendChild(resourceRow(r));
            });
            details.appendChild(list);
            article.appendChild(details);
        }
        return article;
    }

    function render() {
        var status = $('status');
        var main = $('apps');
        main.textContent = '';

        if (state.error) {
            status.className = 'status error';
            status.textContent = state.error;
            status.hidden = false;
            return;
        }

        var filter = state.filter.toLowerCase();
        var shown = state.items.filter(function (app) {
            return !filter || app.metadata.name.toLowerCase().indexOf(filter) >= 0;
        });

        status.className = 'status';
        status.hidden = state.loaded && shown.length > 0;
        status.textContent = !state.loaded
            ? 'Loading…'
            : state.items.length === 0
              ? 'No Applications here.'
              : 'Nothing matches that filter.';

        shown
            .sort(function (a, b) {
                return a.metadata.name.localeCompare(b.metadata.name);
            })
            .forEach(function (app) {
                main.appendChild(card(app));
            });
    }

    function watch() {
        if (stopWatching) stopWatching();
        state.loaded = false;
        render();
        stopWatching = sdk.watch(
            { kind: APPS, namespace: state.namespace, interval: 5000 },
            function (items) {
                state.items = items;
                state.loaded = true;
                state.error = '';
                render();
            },
            fail,
        );
    }

    $('filter').addEventListener('input', function (event) {
        state.filter = event.target.value;
        render();
    });

    $('namespace').addEventListener('change', function (event) {
        state.namespace = event.target.value;
        watch();
    });

    sdk.ready()
        .then(function (context) {
            state.readable = context.readable;
            state.write = context.write;
            $('context').textContent = '— ' + context.contextName;
            watch();
            return sdk.namespaces();
        })
        .then(function (names) {
            var select = $('namespace');
            names.forEach(function (name) {
                var option = el('option', '', name);
                option.value = name;
                select.appendChild(option);
            });
        })
        .catch(fail);
})();
