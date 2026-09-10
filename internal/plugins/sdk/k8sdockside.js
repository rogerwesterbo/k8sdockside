/*
 * The k8sdockside plugin bridge, for a plugin's own views.
 *
 * Include it from a view's HTML before your own script:
 *
 *     <script src="/plugin-ui/_sdk/k8sdockside.js"></script>
 *
 * It defines `window.k8sdockside`. Every call is a message to the app, which
 * answers only for the kinds the plugin declares (its requires, views, cards
 * and ui.kinds) and never for Secrets. A patch is shown to the user first and
 * applied only if they say yes.
 *
 * The page runs in a sandboxed frame with no network access of its own: fetch,
 * XHR and websockets are refused by its Content-Security-Policy. Bundle what
 * you need into your own files.
 */
(function () {
    'use strict';

    var PROTOCOL = 'k8sdockside/plugin@1';
    var pending = new Map();
    var listeners = {};
    var nextId = 1;

    function call(method, params) {
        return new Promise(function (resolve, reject) {
            var id = nextId++;
            pending.set(id, { resolve: resolve, reject: reject });
            window.parent.postMessage({ protocol: PROTOCOL, id: id, method: method, params: params || {} }, '*');
        });
    }

    function emit(event, data) {
        (listeners[event] || []).forEach(function (fn) {
            try {
                fn(data);
            } catch (err) {
                console.error('k8sdockside: a "' + event + '" listener failed', err);
            }
        });
    }

    /* The app's theme, as the same custom properties the app's own components
       use: var(--bg), var(--text), var(--accent), var(--ok) and the rest. */
    function applyTheme(theme) {
        if (!theme) return;
        var root = document.documentElement;
        var tokens = theme.tokens || {};
        Object.keys(tokens).forEach(function (name) {
            root.style.setProperty('--' + name, tokens[name]);
        });
        root.style.colorScheme = theme.base === 'light' ? 'light' : 'dark';
        root.dataset.themeBase = theme.base === 'light' ? 'light' : 'dark';
    }

    window.addEventListener('message', function (event) {
        if (event.source !== window.parent) return;
        var msg = event.data;
        if (!msg || msg.protocol !== PROTOCOL) return;

        if (typeof msg.id === 'number') {
            var waiting = pending.get(msg.id);
            if (!waiting) return;
            pending.delete(msg.id);
            if (msg.error !== undefined) waiting.reject(new Error(msg.error));
            else waiting.resolve(msg.result);
            return;
        }
        if (typeof msg.event === 'string') {
            if (msg.event === 'theme') applyTheme(msg.data);
            emit(msg.event, msg.data);
        }
    });

    /* A section sits in a scrolling detail panel, where a frame cannot size
       itself to its content; so the page's height is measured here and told to
       the app whenever it changes. A tab fills its pane and ignores it. */
    function followHeight() {
        var last = 0;
        function measure() {
            var h = Math.ceil(document.documentElement.scrollHeight);
            if (h !== last) {
                last = h;
                call('resize', { height: h });
            }
        }
        if (typeof ResizeObserver === 'function') {
            new ResizeObserver(measure).observe(document.documentElement);
        }
        window.addEventListener('load', measure);
        measure();
    }

    var ready = call('hello').then(function (context) {
        applyTheme(context.theme);
        if (context.sectionId) {
            // The frame's own height is the page's height; a page that sets
            // height: 100% on html/body would otherwise measure itself forever.
            document.documentElement.style.height = 'auto';
            if (document.body) followHeight();
            else document.addEventListener('DOMContentLoaded', followHeight);
        }
        return context;
    });

    window.k8sdockside = {
        /**
         * Resolves once the app has answered, with what this page is looking at:
         * { pluginId, viewId, sectionId, object, contextId, contextName,
         *   readable, write, actions, theme }.
         *
         * `object` is { kind, namespace, name } for a section in an object's
         * detail view, and null for a view that is a tab of its own.
         */
        ready: function () {
            return ready;
        },

        /** The object a section is drawn for, read live. Rejects on a tab. */
        object: function () {
            return ready.then(function (ctx) {
                if (!ctx.object) throw new Error('this page is not drawn for an object');
                return call('get', ctx.object);
            });
        },

        /**
         * Which of this plugin's actions are offered on an object right now --
         * the section's own object unless one is named -- as
         * [{ id, label, icon, tone, confirm, done }]. For drawing a button group
         * with only the buttons that apply.
         */
        actions: function (ref) {
            return call('actions', ref || {});
        },

        /**
         * Runs one of the actions this plugin declares in its manifest, on the
         * section's object or the one named ({ namespace, name }). The app asks
         * the user first, every time. Resolves with { created } -- the name of an
         * object a create request made -- and rejects if they decline.
         */
        run: function (actionId, ref) {
            return call('action', Object.assign({}, ref || {}, { id: actionId }));
        },

        /** Sets a section's height by hand, for a page that lays itself out oddly. */
        resize: function (height) {
            return call('resize', { height: height });
        },

        /** Objects of one kind: { kind, namespace?, selector? } -> object[]. */
        list: function (query) {
            return call('list', query);
        },

        /** One object: { kind, namespace, name } -> object. */
        get: function (ref) {
            return call('get', ref);
        },

        /**
         * Polls `list` and calls back with the objects whenever asked, and once
         * straight away. Returns a function that stops it.
         * { kind, namespace?, selector?, interval? (ms, default 5000) }
         */
        watch: function (query, callback, onError) {
            var stopped = false;
            var timer = null;
            var interval = Math.max(1000, (query && query.interval) || 5000);
            function tick() {
                call('list', query)
                    .then(function (items) {
                        if (!stopped) callback(items);
                    })
                    .catch(function (err) {
                        if (!stopped && onError) onError(err);
                    })
                    .then(function () {
                        if (!stopped) timer = setTimeout(tick, interval);
                    });
            }
            tick();
            return function () {
                stopped = true;
                if (timer) clearTimeout(timer);
            };
        },

        /** The namespaces of this cluster. */
        namespaces: function () {
            return call('namespaces');
        },

        /**
         * Asks to merge-patch one object: { kind, namespace, name, patch }.
         * `patch` is an object or JSON/YAML text. Needs "ui": { "write": true },
         * and the user sees the patch and confirms it first. Rejects if they
         * decline.
         */
        patch: function (request) {
            var params = Object.assign({}, request);
            if (typeof params.patch !== 'string') params.patch = JSON.stringify(params.patch, null, 2);
            return call('patch', params);
        },

        /** Opens an object in the details panel, or a kind's own tab if no name is given. */
        open: function (ref) {
            return call('open', ref);
        },

        /** Opens another of this plugin's views by id, or 'overview'. */
        openView: function (viewId) {
            return call('openView', { viewId: viewId });
        },

        /** Opens an object in the YAML editor. */
        edit: function (ref) {
            return call('edit', ref);
        },

        /** Opens the logs of a pod (or anything else the log view accepts). */
        logs: function (ref) {
            return call('logs', ref);
        },

        /** Opens an http(s) address in the user's browser. */
        openUrl: function (url) {
            return call('openUrl', { url: url });
        },

        /** Listens for pushes from the app. Events: 'theme'. Returns an unsubscribe function. */
        on: function (event, fn) {
            (listeners[event] = listeners[event] || []).push(fn);
            return function () {
                listeners[event] = (listeners[event] || []).filter(function (f) {
                    return f !== fn;
                });
            };
        },
    };
})();
