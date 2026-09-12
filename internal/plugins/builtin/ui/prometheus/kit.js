// The drawing kit the Prometheus pages share: elements, icons, chips and
// buttons, a sparkline, a ring and a stacked bar.
//
// Everything that came from the cluster is written with textContent, never
// innerHTML: the frame is sandboxed, but a page that let a rule's annotation
// run as markup would be handing that text the bridge.
(function () {
    'use strict';

    var SVG = 'http://www.w3.org/2000/svg';

    function el(tag, className, text) {
        var node = document.createElement(tag);
        if (className) node.className = className;
        if (text !== undefined && text !== null) node.textContent = text;
        return node;
    }

    function add(parent) {
        for (var i = 1; i < arguments.length; i++) {
            var child = arguments[i];
            if (child === null || child === undefined || child === false) continue;
            parent.appendChild(typeof child === 'string' ? document.createTextNode(child) : child);
        }
        return parent;
    }

    function svg(tag, attrs) {
        var node = document.createElementNS(SVG, tag);
        Object.keys(attrs || {}).forEach(function (k) {
            node.setAttribute(k, attrs[k]);
        });
        return node;
    }

    // Single-stroke icons on a 24-unit grid.
    var ICONS = {
        logo: ['M12 2.5c.8 2.6 3.8 4 3.8 7.4a3.8 3.8 0 0 1-7.6 0c0-1.9 1-3 1.5-3.9.4 1.4 1.3 1.9 2.2 1.9-.4-1.8.1-3.6.1-5.4z', 'M6 17h12', 'M7.5 20.5h9'],
        bell: ['M6 16v-5a6 6 0 0 1 12 0v5l1.5 2h-15z', 'M10 21h4'],
        alert: ['M12 3l10 18H2z', 'M12 10v4', 'M12 17.5h.01'],
        check: ['M4 12.5l5 5L20 6.5'],
        close: ['M6 6l12 12', 'M18 6L6 18'],
        arrow: ['M5 12h14', 'M13 6l6 6-6 6'],
        open: ['M14 4h6v6', 'M20 4l-9 9', 'M18 14v6H4V6h6'],
        edit: ['M4 20h4L19 9l-4-4L4 16z', 'M14 6l4 4'],
        target: ['M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z', 'M12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8z', 'M12 12h.01'],
        server: ['M4 4h16v6H4z', 'M4 14h16v6H4z', 'M8 7h.01', 'M8 17h.01'],
        clock: ['M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z', 'M12 7v5l3 2'],
        chart: ['M4 4v16h16', 'M8 15l3-4 3 2 5-6'],
        activity: ['M3 12h4l3-8 4 16 3-8h4'],
        layers: ['M12 3l9 5-9 5-9-5z', 'M3 13l9 5 9-5'],
        disk: ['M4 6c0-1.7 3.6-3 8-3s8 1.3 8 3-3.6 3-8 3-8-1.3-8-3z', 'M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6', 'M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3'],
        rule: ['M5 3h14v18H5z', 'M9 8h6', 'M9 12h6', 'M9 16h3'],
        plus: ['M12 5v14', 'M5 12h14'],
        trash: ['M4 7h16', 'M9 7V4h6v3', 'M6 7l1 13h10l1-13'],
        copy: ['M8 8h12v12H8z', 'M16 8V4H4v12h4'],
        search: ['M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14z', 'M20 20l-4-4'],
        share: ['M6 12a2.5 2.5 0 1 0 0-.01', 'M18 6a2.5 2.5 0 1 0 0-.01', 'M18 18a2.5 2.5 0 1 0 0-.01', 'M8.2 10.8l7.6-3.6', 'M8.2 13.2l7.6 3.6'],
        box: ['M12 3l8 4.5v9L12 21l-8-4.5v-9z', 'M12 12l8-4.5', 'M12 12v9', 'M12 12L4 7.5'],
        sliders: ['M4 7h10', 'M18 7h2', 'M4 17h4', 'M12 17h8', 'M16 5v4', 'M10 15v4'],
        probe: ['M12 21a9 9 0 1 0 0-18', 'M12 12l6-6', 'M12 12h.01'],
    };

    function icon(name, className) {
        var node = svg('svg', { viewBox: '0 0 24 24', class: 'ico' + (className ? ' ' + className : ''), 'aria-hidden': 'true' });
        (ICONS[name] || ICONS.box).forEach(function (d) {
            node.appendChild(svg('path', { d: d }));
        });
        return node;
    }

    function chip(text, tone, iconName, title) {
        var node = el('span', 'chip' + (tone ? ' ' + tone : ''));
        if (iconName) node.appendChild(icon(iconName));
        node.appendChild(el('span', '', text));
        if (title) node.title = title;
        return node;
    }

    function button(text, className, iconName, onClick) {
        var node = el('button', className || '');
        node.type = 'button';
        if (iconName) node.appendChild(icon(iconName));
        if (text) node.appendChild(el('span', '', text));
        if (onClick) node.addEventListener('click', onClick);
        return node;
    }

    function link(text, onClick, title) {
        var node = el('button', 'link', text);
        node.type = 'button';
        if (title) node.title = title;
        node.addEventListener('click', onClick);
        return node;
    }

    // What the plugin is about, from its manifest's links, each opened in the
    // user's browser. Null when the app did not say or there are none.
    function about(sdk, plugin, onError) {
        if (!plugin || !plugin.links || !plugin.links.length) return null;
        var row = el('div', 'about');
        row.appendChild(el('span', 'about-label', plugin.version ? 'Plugin ' + plugin.version + ' ·' : 'About'));
        plugin.links.forEach(function (l) {
            row.appendChild(
                link(
                    l.label,
                    function () {
                        sdk.openUrl(l.url).catch(onError || function () {});
                    },
                    l.url,
                ),
            );
        });
        return row;
    }

    function plural(n, one, many) {
        return n + ' ' + (n === 1 ? one : many || one + 's');
    }

    // A small filled line of one series, scaled from zero: these are counts,
    // rates and sizes, where the distance from nothing is what is read.
    function spark(points, tone) {
        var node = svg('svg', { viewBox: '0 0 100 32', preserveAspectRatio: 'none', class: 'spark' + (tone ? ' ' + tone : ''), 'aria-hidden': 'true' });
        var pts = (points || []).filter(function (p) {
            return isFinite(p.v);
        });
        if (pts.length < 2) return node;
        var t0 = pts[0].t;
        var span = pts[pts.length - 1].t - t0 || 1;
        var max = 0;
        pts.forEach(function (p) {
            if (p.v > max) max = p.v;
        });
        var top = max > 0 ? max * 1.1 : 1;
        var d = pts
            .map(function (p, i) {
                return (i ? 'L' : 'M') + (((p.t - t0) / span) * 100).toFixed(2) + ' ' + (31 - (p.v / top) * 29).toFixed(2);
            })
            .join(' ');
        node.appendChild(svg('path', { d: d + ' L100 32 L0 32 Z', class: 'spark-area' }));
        node.appendChild(svg('path', { d: d, class: 'spark-line' }));
        return node;
    }

    // A ring of counts with a number in the middle. segments: [{ count, tone }]
    function ring(segments, big, small) {
        var total = segments.reduce(function (s, x) {
            return s + x.count;
        }, 0);
        var wrap = el('div', 'ring');
        var r = 46;
        var C = 2 * Math.PI * r;
        var node = svg('svg', { viewBox: '0 0 120 120', class: 'ring-svg', 'aria-hidden': 'true' });
        node.appendChild(svg('circle', { cx: 60, cy: 60, r: r, class: 'ring-track' }));
        var offset = 0;
        segments.forEach(function (seg) {
            if (!seg.count || !total) return;
            var len = (seg.count / total) * C;
            node.appendChild(
                svg('circle', {
                    cx: 60,
                    cy: 60,
                    r: r,
                    class: 'ring-seg ' + seg.tone,
                    'stroke-dasharray': len.toFixed(2) + ' ' + (C - len).toFixed(2),
                    'stroke-dashoffset': (-offset).toFixed(2),
                    transform: 'rotate(-90 60 60)',
                }),
            );
            offset += len;
        });
        wrap.appendChild(node);
        var mid = el('div', 'ring-mid');
        add(mid, el('strong', '', big), el('span', '', small));
        wrap.appendChild(mid);
        return wrap;
    }

    // A stacked bar of counts, each segment labelled underneath.
    // segments: [{ label, count, tone }]
    function stackBar(segments) {
        var total = segments.reduce(function (s, x) {
            return s + x.count;
        }, 0);
        var wrap = el('div', 'stack');
        var bar = el('div', 'stack-bar');
        var keys = el('div', 'stack-keys');
        segments.forEach(function (seg) {
            if (!seg.count) return;
            var part = el('i', 'seg ' + seg.tone);
            part.style.flexGrow = String(seg.count);
            part.title = seg.label + ': ' + seg.count;
            bar.appendChild(part);
            var key = el('span', 'stack-key');
            add(key, el('i', 'dot ' + seg.tone), el('span', '', seg.label), el('strong', '', String(seg.count)));
            keys.appendChild(key);
        });
        if (!total) bar.appendChild(el('i', 'seg muted empty'));
        add(wrap, bar, keys);
        return wrap;
    }

    // Asks the app to apply a patch. The app shows it to the user first; a
    // "no" is not an error worth showing.
    function apply(sdk, ref, patch) {
        return sdk.patch({ kind: ref.kind, namespace: ref.namespace, name: ref.name, patch: patch }).then(
            function () {
                return true;
            },
            function (err) {
                if (/declined/.test(err.message)) return false;
                throw err;
            },
        );
    }

    // ----- the grip ----------------------------------------------------------

    // A grab strip on the inner edge of a panel on the right: drag it, or
    // focus it and use the arrow keys, to make the panel wider or narrower; a
    // double-click puts it back. The width is a custom property on the root,
    // so the panel and whatever makes room for it read the one value -- unset,
    // the stylesheet's own width stands. The page places the strip, and keeps
    // the width wherever it keeps the rest of its state.
    // opts: { panel, prop, min, room, initial, label, className, onResize(px, done) }
    //   room      what the panel always leaves of the window
    //   onResize  px is 0 once the panel is back to the stylesheet's width;
    //             done is false while a drag goes on, true when it ends
    function grip(opts) {
        var root = document.documentElement;
        var node = el('div', 'grip' + (opts.className ? ' ' + opts.className : ''));
        node.tabIndex = 0;
        node.title = 'Drag to resize · double-click to reset';
        node.setAttribute('role', 'separator');
        node.setAttribute('aria-orientation', 'vertical');
        node.setAttribute('aria-label', opts.label || 'Resize the panel');
        var wanted = opts.initial > 0 ? opts.initial : 0;
        var drag = null;
        var frame = 0;

        function fit(px) {
            return Math.round(Math.max(opts.min, Math.min(px, window.innerWidth - opts.room)));
        }

        // The width asked for is kept as asked: a smaller window takes what it
        // must from the panel and gives it back when it grows again.
        function apply() {
            if (wanted) root.style.setProperty(opts.prop, fit(wanted) + 'px');
            else root.style.removeProperty(opts.prop);
        }

        // While a drag goes on the page hears of it once a frame at most.
        function tell(done) {
            if (!opts.onResize) return;
            cancelAnimationFrame(frame);
            frame = 0;
            if (done) opts.onResize(wanted, true);
            else
                frame = requestAnimationFrame(function () {
                    frame = 0;
                    opts.onResize(wanted, false);
                });
        }

        function resize(px, done) {
            wanted = px ? fit(px) : 0;
            apply();
            tell(done);
        }

        function release(event) {
            if (!drag) return;
            drag = null;
            document.body.classList.remove('gripping');
            if (node.hasPointerCapture(event.pointerId)) node.releasePointerCapture(event.pointerId);
            tell(true);
        }

        node.addEventListener('pointerdown', function (event) {
            if (event.button !== 0) return;
            event.preventDefault();
            node.setPointerCapture(event.pointerId);
            drag = { x: event.clientX, from: opts.panel.getBoundingClientRect().width };
            document.body.classList.add('gripping');
        });
        node.addEventListener('pointermove', function (event) {
            // The panel is on the right: it grows as the pointer goes left.
            if (drag) resize(drag.from + drag.x - event.clientX, false);
        });
        node.addEventListener('pointerup', release);
        node.addEventListener('pointercancel', release);
        node.addEventListener('dblclick', function () {
            resize(0, true);
        });
        node.addEventListener('keydown', function (event) {
            var step = event.shiftKey ? 48 : 16;
            var now = wanted || opts.panel.getBoundingClientRect().width;
            if (event.key === 'ArrowLeft') resize(now + step, true);
            else if (event.key === 'ArrowRight') resize(now - step, true);
            else return;
            event.preventDefault();
        });
        window.addEventListener('resize', function () {
            if (!wanted) return;
            apply();
            tell(false);
        });
        apply();
        return node;
    }

    window.PromKit = {
        el: el,
        add: add,
        svg: svg,
        icon: icon,
        chip: chip,
        button: button,
        link: link,
        about: about,
        plural: plural,
        spark: spark,
        ring: ring,
        stackBar: stackBar,
        apply: apply,
        grip: grip,
    };
})();
