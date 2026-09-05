import { mount } from 'svelte'
import App from './App.svelte'
import { restoreTheme } from './lib/theme/apply'

// Before the app mounts, not after: the theme lives in a settings file the Go
// side has to be asked for, and the first frames are drawn long before that
// answer arrives. Repainting from the cache here is what stops a user on a
// light theme seeing a flash of dark at every launch. See restoreTheme.
restoreTheme()

mount(App, { target: document.getElementById('app')! })
