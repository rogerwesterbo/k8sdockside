// The boundary between the plugin bindings and the rest of the app, matching
// state/adopt.ts and theme/adopt.ts: the generated types are nullable wherever
// a Go slice could be nil, and resolving that once here keeps `?? []` out of
// the sidebar and the overview.

import type * as bindings from '../../../bindings/github.com/rogerwesterbo/k8sdockside/internal/plugins/models.js';
import type { Plugin, PluginCatalogue, PluginSummary } from './types';

export function adoptPlugin(plugin: bindings.Plugin): Plugin {
    return {
        id: plugin.id,
        name: plugin.name,
        tagline: plugin.tagline ?? '',
        icon: plugin.icon || 'puzzle',
        author: plugin.author ?? '',
        docs: plugin.docs ?? '',
        description: plugin.description ?? '',
        requires: (plugin.requires ?? []).map((req) => ({
            kind: req.kind,
            label: req.label || req.kind,
            optional: req.optional ?? false,
        })),
        views: (plugin.views ?? []).map((view) => ({
            id: view.id,
            label: view.label,
            icon: view.icon || 'puzzle',
            type: view.type ?? 'table',
            kind: view.kind ?? '',
            namespace: view.namespace ?? '',
            selector: view.selector ?? '',
        })),
        origin: plugin.origin,
        pack: plugin.pack,
        disabled: plugin.disabled ?? false,
    };
}

export function adoptPluginCatalogue(catalogue: bindings.Catalogue): PluginCatalogue {
    return {
        plugins: (catalogue.plugins ?? []).map(adoptPlugin),
        dir: catalogue.dir ?? '',
        folders: [...(catalogue.folders ?? [])],
        problems: (catalogue.problems ?? []).map((p) => ({ path: p.path, message: p.message })),
    };
}

export function adoptPluginSummary(summary: bindings.Summary): PluginSummary {
    return {
        pluginId: summary.pluginId,
        installed: summary.installed,
        checked: summary.checked,
        requirements: (summary.requirements ?? []).map((req) => ({
            kind: req.kind,
            label: req.label || req.kind,
            optional: req.optional,
            served: req.served,
            error: req.error,
        })),
        cards: (summary.cards ?? []).map((card) => ({
            label: card.label,
            kind: card.kind,
            total: card.total,
            grouped: card.grouped,
            buckets: (card.buckets ?? []).map((b) => ({ value: b.value, count: b.count, tone: b.tone })),
            error: card.error,
        })),
        error: summary.error,
    };
}
