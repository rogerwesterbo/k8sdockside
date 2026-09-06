// Which section the settings tab opens on.
//
// Module state rather than component state, because the shell rebuilds the
// settings view every time it is returned to -- see SettingsView -- and a
// link from elsewhere (the help page's "open plugin settings") wants to land
// on one section rather than wherever the user last was. Deliberately not
// persisted: which page of settings was open is where you were a moment ago,
// not a preference.

export const settingsSection = $state({ current: 'appearance' });

/** Chooses the section the settings tab shows next. */
export function rememberSection(id: string): void {
    settingsSection.current = id;
}
