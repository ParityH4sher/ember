<script lang="ts" module>
  // 24×24 stroke icons, drawn inline so that the app loads no icon font.
  const paths = {
    home: 'M3 11 12 3l9 8M5 10v10h5v-6h4v6h5V10',
    desktop: 'M3 4h18v12H3zM8 20h8M12 16v4',
    notes: 'M6 3h9l4 4v14H6zM14 3v5h5M9 12h7M9 16h7',
    plugin: 'M9 3v4M15 3v4M7 7h10v5a5 5 0 0 1-10 0zM12 17v4',
    settings: 'M4 6h10M18 6h2M4 12h4M12 12h8M4 18h12M20 18h0M16 4v4M10 10v4M16 16v4',
    user: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21a8 8 0 0 1 16 0',
    shield: 'M12 3 4 6v6c0 5 3.5 8 8 9 4.5-1 8-4 8-9V6z M9 12l2 2 4-4',
    menu: 'M4 6h16M4 12h16M4 18h16',
    logout: 'M15 4h4v16h-4M10 8l-4 4 4 4M6 12h10',
    external: 'M14 4h6v6M20 4l-9 9M18 14v6H4V6h6',
    apps: 'M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z',
    search: 'M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14zM16 16l5 5',
    pin: 'M9 4h6M10 4v6l-3 4h10l-3-4V4M12 14v7',
    info: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM12 11v6M12 7.5v.5',
    close: 'M6 6l12 12M18 6 6 18',
    plus: 'M12 5v14M5 12h14'
  } as const;
  export type IconName = keyof typeof paths;
</script>

<script lang="ts">
  let { name, size = 20 }: { name: IconName; size?: number } = $props();
</script>

<svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
  stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  <path d={paths[name]} />
</svg>

<style>
  svg { flex: none; vertical-align: middle }
</style>
