/**
 * Built-in theme presets shipped with the plugin (PRD P8).
 *
 * Each preset is a plain CSS string pushed verbatim to POST /api/theme.
 * Custom CSS is allowed too — self-hosted, single owner, so the server
 * accepts arbitrary CSS by design.
 */

export interface ThemePreset {
  id: string;
  name: string;
  css: string;
}

export const THEME_PRESETS: readonly ThemePreset[] = [
  {
    id: "default",
    name: "Default",
    css: `/* Default — the server's built-in stylesheet. Push an empty theme to reset. */
`,
  },
  {
    id: "serif-dark",
    name: "Serif Dark",
    css: `/* Serif Dark */
:root {
  --op-bg: #14161a;
  --op-fg: #d6d3cd;
  --op-accent: #c9a66b;
  --op-muted: #8a8f98;
}
body {
  background: var(--op-bg);
  color: var(--op-fg);
  font-family: Georgia, "Times New Roman", serif;
  line-height: 1.7;
  max-width: 42rem;
  margin: 3rem auto;
  padding: 0 1.25rem;
}
h1, h2, h3 { color: var(--op-accent); font-weight: 600; }
a { color: var(--op-accent); }
code, pre { background: #1e2127; border-radius: 4px; }
blockquote { border-left: 3px solid var(--op-accent); color: var(--op-muted); margin-left: 0; padding-left: 1rem; }
img { max-width: 100%; }
`,
  },
  {
    id: "warm",
    name: "Warm",
    css: `/* Warm */
:root {
  --op-bg: #faf3e7;
  --op-fg: #3d2f24;
  --op-accent: #b25b2e;
  --op-muted: #8c7a6b;
}
body {
  background: var(--op-bg);
  color: var(--op-fg);
  font-family: "Avenir Next", "Segoe UI", sans-serif;
  line-height: 1.65;
  max-width: 40rem;
  margin: 3rem auto;
  padding: 0 1.25rem;
}
h1, h2, h3 { color: var(--op-accent); }
a { color: var(--op-accent); }
code, pre { background: #f0e4d0; border-radius: 4px; }
blockquote { border-left: 3px solid var(--op-accent); color: var(--op-muted); margin-left: 0; padding-left: 1rem; }
img { max-width: 100%; }
`,
  },
  {
    id: "mono",
    name: "Mono",
    css: `/* Mono */
:root {
  --op-bg: #ffffff;
  --op-fg: #111111;
  --op-accent: #111111;
  --op-muted: #6e6e6e;
}
body {
  background: var(--op-bg);
  color: var(--op-fg);
  font-family: "SF Mono", "Cascadia Code", Consolas, monospace;
  font-size: 0.95rem;
  line-height: 1.6;
  max-width: 38rem;
  margin: 3rem auto;
  padding: 0 1.25rem;
}
h1, h2, h3 { font-weight: 700; }
a { color: var(--op-fg); text-decoration: underline; }
code, pre { background: #f2f2f2; border-radius: 3px; }
blockquote { border-left: 3px solid var(--op-fg); color: var(--op-muted); margin-left: 0; padding-left: 1rem; }
img { max-width: 100%; border: 1px solid #e0e0e0; }
`,
  },
];
