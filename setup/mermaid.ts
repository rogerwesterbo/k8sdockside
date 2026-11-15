import { defineMermaidSetup } from '@slidev/types'

// Diagrams in the K8s Dockside palette instead of Mermaid's grey dark theme
export default defineMermaidSetup(() => ({
  theme: 'base',
  themeVariables: {
    darkMode: true,
    background: '#0d2434',
    primaryColor: '#123044',
    mainBkg: '#123044',
    nodeBorder: '#62c8b4',
    nodeTextColor: '#e2e8f0',
    primaryTextColor: '#e2e8f0',
    primaryBorderColor: '#62c8b4',
    secondaryColor: '#0f2a3a',
    tertiaryColor: '#0f2a3a',
    clusterBkg: 'rgba(15, 42, 58, 0.6)',
    clusterBorder: '#4a9bd4',
    lineColor: '#62c8b4',
    edgeLabelBackground: '#0d2434',
  },
}))
