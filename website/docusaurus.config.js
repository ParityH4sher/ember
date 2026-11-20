/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'Ember',
  tagline: 'Project documentation and planning',
  url: process.env.DOCS_URL || 'http://localhost:3000',
  baseUrl: process.env.DOCS_BASE_URL || '/',
  trailingSlash: true,
  staticDirectories: ['static', '../contracts'],
  clientModules: ['./src/clientModules/d3-transition.js'],
  onBrokenLinks: 'throw',
  onBrokenAnchors: 'throw',
  onDuplicateRoutes: 'throw',
  markdown: {
    format: 'detect',
    mermaid: true,
    hooks: {
      onBrokenMarkdownLinks: 'throw',
    },
  },
  presets: [
    [
      'classic',
      {
        docs: {
          path: '../docs',
          routeBasePath: '/',
          sidebarPath: './sidebars.js',
        },
        blog: false,
        pages: false,
      },
    ],
  ],
  themes: ['@docusaurus/theme-mermaid'],
  themeConfig: {
    navbar: {
      title: 'Ember',
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docs',
          label: 'Documentation',
          position: 'left',
        },
        {
          href: 'https://github.com/ParityH4sher/ember',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    colorMode: {
      respectPrefersColorScheme: true,
    },
    docs: {
      sidebar: {
        hideable: true,
      },
    },
    prism: {
      additionalLanguages: ['bash', 'json', 'yaml'],
    },
  },
};

export default config;
