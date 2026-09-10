// A virtual machine's own panel: its state, where it runs, and a button group
// of the lifecycle actions this plugin declares -- the same ones the app puts
// on the action bar, drawn the way Headlamp's KubeVirt plugin draws them.
//
// Plain script, no build step. Everything from the cluster is written with
// textContent, never innerHTML.
(function () {
    'use strict';

    var sdk = window.k8sdockside;
    var VMIS = 'crd:virtualmachineinstances.kubevirt.io';
    var MIGRATIONS = 'crd:virtualmachineinstancemigrations.kubevirt.io';
    var POLL = 4000;

    // The app's icon names, as the same single-stroke paths on a 24x24 grid.
    var ICONS = {
        play: ['M7 4.5v15l12-7.5z'],
        pause: ['M7 5h3.5v14H7z', 'M13.5 5H17v14h-3.5z'],
        stop: ['M6 6h12v12H6z'],
        power: ['M12 3v8', 'M6.3 6.3a8 8 0 1 0 11.4 0'],
        repeat: ['M17 2l4 4-4 4', 'M3 11V9a4 4 0 0 1 4-4h14', 'M7 22l-4-4 4-4', 'M21 13v2a4 4 0 0 1-4 4H3'],
        forward: ['M4 8h5v8H4z', 'M15 8h5v8h-5z', 'M9 12h6', 'M13 10l2 2-2 2'],
        dot: ['M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6z'],
    };

    var ctx = null;
    var busy = false;

    var $ = function (id) {
        return document.getElementById(id);
    };

    function el(tag, className, text) {
        var node = document.createElement(tag);
        if (className) node.className = className;
        if (text !== undefined) node.textContent = text;
        return node;
    }

    function icon(name) {
        var ns = 'http://www.w3.org/2000/svg';
        var svg = document.createElementNS(ns, 'svg');
        svg.setAttribute('viewBox', '0 0 24 24');
        (ICONS[name] || ICONS.dot).forEach(function (d) {
            var path = document.createElementNS(ns, 'path');
            path.setAttribute('d', d);
            svg.appendChild(path);
        });
        return svg;
    }

    function dig(obj, path) {
        return path.split('.').reduce(function (at, key) {
            return at && at[key] !== undefined ? at[key] : undefined;
        }, obj);
    }

    function condition(obj, type) {
        var list = dig(obj, 'status.conditions') || [];
        for (var i = 0; i < list.length; i++) if (list[i].type === type) return list[i];
        return null;
    }

    function age(timestamp) {
        if (!timestamp) return '';
        var seconds = Math.max(0, (Date.now() - new Date(timestamp).getTime()) / 1000);
        if (seconds < 90) return Math.round(seconds) + 's';
        if (seconds < 5400) return Math.round(seconds / 60) + 'm';
        if (seconds < 172800) return Math.round(seconds / 3600) + 'h';
        return Math.round(seconds / 86400) + 'd';
    }

    function tone(status) {
        switch (status) {
            case 'Running':
                return 'ok';
            case 'Paused':
            case 'Starting':
            case 'Stopping':
            case 'Migrating':
            case 'Provisioning':
                return 'warn';
            case 'Stopped':
            case '':
                return '';
            default:
                return 'error';
        }
    }

    // ----- what the machine is ----------------------------------------------

    function cpuOf(vm) {
        var cpu = dig(vm, 'spec.template.spec.domain.cpu') || {};
        var total = (cpu.cores || 1) * (cpu.sockets || 1) * (cpu.threads || 1);
        return total + (total === 1 ? ' vCPU' : ' vCPUs');
    }

    function memoryOf(vm) {
        return (
            dig(vm, 'spec.template.spec.domain.memory.guest') ||
            dig(vm, 'spec.template.spec.domain.resources.requests.memory') ||
            '—'
        );
    }

    function addFact(list, label, value, onClick) {
        list.appendChild(el('dt', '', label));
        var dd = el('dd');
        if (onClick && value) {
            var link = el('button', 'link', value);
            link.onclick = onClick;
            dd.appendChild(link);
        } else {
            dd.textContent = value || '—';
        }
        list.appendChild(dd);
    }

    function drawFacts(vm, vmi, pod) {
        var facts = $('facts');
        facts.textContent = '';
        var ns = vm.metadata.namespace;

        addFact(facts, 'CPU', cpuOf(vm));
        addFact(facts, 'Memory', memoryOf(vm));
        addFact(facts, 'Run strategy', dig(vm, 'spec.runStrategy') || (dig(vm, 'spec.running') ? 'Always (running: true)' : 'Halted (running: false)'));

        if (!vmi) {
            addFact(facts, 'Instance', 'none — the machine is not running');
            return;
        }
        var node = dig(vmi, 'status.nodeName');
        addFact(facts, 'Node', node, function () {
            sdk.open({ kind: 'nodes', namespace: '', name: node });
        });
        var ips = (dig(vmi, 'status.interfaces') || [])
            .map(function (i) {
                return i.ipAddress ? (i.name ? i.name + ': ' : '') + i.ipAddress : '';
            })
            .filter(Boolean);
        addFact(facts, 'IP addresses', ips.join(', '));
        addFact(facts, 'Guest OS', dig(vmi, 'status.guestOSInfo.prettyName') || dig(vmi, 'status.guestOSInfo.name') || 'unknown — no guest agent');
        addFact(facts, 'Instance', vmi.metadata.name, function () {
            sdk.open({ kind: VMIS, namespace: ns, name: vmi.metadata.name });
        });
        if (pod) {
            addFact(facts, 'Launcher pod', pod.metadata.name, function () {
                sdk.open({ kind: 'pods', namespace: ns, name: pod.metadata.name });
            });
        }
        var migration = dig(vmi, 'status.migrationState');
        if (migration && !migration.completed) {
            addFact(facts, 'Migrating', (migration.sourceNode || '?') + ' → ' + (migration.targetNode || '?'));
        }
    }

    function drawConditions(vm) {
        var list = dig(vm, 'status.conditions') || [];
        var box = $('conditions');
        box.textContent = '';
        list.forEach(function (c) {
            var chip = el('span', 'chip' + (c.status === 'True' ? '' : ' bad'), c.type + ': ' + c.status);
            if (c.message || c.reason) chip.title = c.message || c.reason;
            box.appendChild(chip);
        });
        $('conditions-block').hidden = list.length === 0;
    }

    function drawMigrations(items, name) {
        var mine = (items || [])
            .filter(function (m) {
                return dig(m, 'spec.vmiName') === name;
            })
            .sort(function (a, b) {
                return (b.metadata.creationTimestamp || '').localeCompare(a.metadata.creationTimestamp || '');
            })
            .slice(0, 5);
        var body = $('migrations');
        body.textContent = '';
        mine.forEach(function (m) {
            var row = el('tr');
            var state = dig(m, 'status.migrationState') || {};
            [m.metadata.name, dig(m, 'status.phase') || '', state.sourceNode || '', state.targetNode || ''].forEach(function (v) {
                row.appendChild(el('td', '', v));
            });
            body.appendChild(row);
        });
        $('migrations-block').hidden = mine.length === 0;
    }

    // ----- the button group --------------------------------------------------

    function drawButtons(offered) {
        var box = $('buttons');
        box.textContent = '';
        offered.forEach(function (action) {
            var button = el('button', action.tone === 'danger' ? 'danger' : '');
            button.title = action.label;
            button.setAttribute('aria-label', action.label);
            button.disabled = busy;
            button.appendChild(icon(action.icon));
            button.onclick = function () {
                run(action);
            };
            box.appendChild(button);
        });
    }

    function run(action) {
        busy = true;
        refresh();
        // The app asks the user before anything is sent.
        sdk.run(action.id)
            .then(function () {
                showError('');
            })
            .catch(function (err) {
                if (!/declined/.test(err.message)) showError(err.message);
            })
            .then(function () {
                busy = false;
                refresh();
            });
    }

    function showError(text) {
        $('error').textContent = text;
        $('error').hidden = !text;
    }

    // ----- reading it all ----------------------------------------------------

    function quietly(promise) {
        return promise.catch(function () {
            return null;
        });
    }

    function refresh() {
        var ref = ctx.object;
        return Promise.all([
            sdk.object(),
            quietly(sdk.get({ kind: VMIS, namespace: ref.namespace, name: ref.name })),
            quietly(sdk.list({ kind: 'pods', namespace: ref.namespace, selector: 'vm.kubevirt.io/name=' + ref.name })),
            quietly(sdk.list({ kind: MIGRATIONS, namespace: ref.namespace })),
            quietly(sdk.actions()),
        ])
            .then(function (got) {
                var vm = got[0];
                var vmi = got[1];
                var pods = (got[2] || []).filter(function (p) {
                    return dig(p, 'status.phase') === 'Running';
                });
                var status = dig(vm, 'status.printableStatus') || '';

                var pill = $('status');
                pill.textContent = status || 'Unknown';
                pill.className = 'pill ' + tone(status);
                $('since').textContent = 'created ' + age(vm.metadata.creationTimestamp) + ' ago';

                drawButtons(got[4] || []);
                drawFacts(vm, vmi, pods[0] || null);
                drawConditions(vm);
                drawMigrations(got[3], ref.name);
            })
            .catch(function (err) {
                showError(err.message);
            });
    }

    sdk.ready().then(function (context) {
        ctx = context;
        if (!ctx.object) {
            showError('This page is a detail panel, drawn for one virtual machine.');
            return;
        }
        refresh();
        setInterval(function () {
            if (!busy) refresh();
        }, POLL);
    });
})();
