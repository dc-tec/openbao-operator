import fs from 'node:fs';
import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

const repoUrl = 'https://github.com/dc-tec/openbao-operator';
const docsEditBase = `${repoUrl}/edit/main/docs/`;
const contributeEditBase = `${repoUrl}/edit/main/docs/contribute/`;
const docsPluginDefaultExclude = [
  '**/_*.{js,jsx,ts,tsx,md,mdx}',
  '**/_*/**',
  '**/*.test.{js,jsx,ts,tsx}',
  '**/__tests__/**',
];
const releaseLineVersionLabels = {
  '0.2.0': '0.2.x',
  '0.1.0': '0.1.x',
} as const;

function readDocsVersions(): string[] {
  try {
    const versions = JSON.parse(fs.readFileSync(new URL('./versions.json', import.meta.url), 'utf8'));
    return Array.isArray(versions) ? versions : [];
  } catch {
    return [];
  }
}

const docsVersions = new Set(readDocsVersions());
const releaseLineVersions = Object.fromEntries(
  Object.entries(releaseLineVersionLabels)
    .filter(([version]) => docsVersions.has(version))
    .map(([version, label]) => [version, {label}]),
);

const config: Config = {
  title: 'OpenBao Operator',
  tagline: 'Operator-first docs for secure OpenBao lifecycle management on Kubernetes.',
  favicon: 'img/brand/favicon.png',

  future: {
    v4: {
      removeLegacyPostBuildHeadAttribute: true,
      useCssCascadeLayers: true,
      siteStorageNamespacing: true,
      fasterByDefault: true,
      mdx1CompatDisabledByDefault: false,
    },
  },

  url: 'https://dc-tec.github.io',
  baseUrl: '/openbao-operator/',
  organizationName: 'dc-tec',
  projectName: 'openbao-operator',

  onBrokenLinks: 'throw',
  onDuplicateRoutes: 'throw',

  markdown: {
    mermaid: true,
    hooks: {
      onBrokenMarkdownLinks: 'throw',
    },
  },
  themes: ['@docusaurus/theme-mermaid'],

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          path: '../docs',
          routeBasePath: 'docs',
          sidebarPath: './sidebars.ts',
          exclude: [...docsPluginDefaultExclude, '**/contribute/**'],
          editUrl: ({docPath}) => `${docsEditBase}${docPath}`,
          showLastUpdateAuthor: true,
          showLastUpdateTime: true,
          versions: {
            current: {
              label: 'next',
              path: 'next',
            },
            ...releaseLineVersions,
          },
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
        sitemap: {
          changefreq: 'weekly',
          priority: 0.6,
          ignorePatterns: ['/tags/**'],
        },
      } satisfies Preset.Options,
    ],
  ],

  plugins: [
    [
      '@docusaurus/plugin-content-docs',
      {
        id: 'contribute',
        path: '../.contribute-docs',
        routeBasePath: 'contribute',
        sidebarPath: './sidebarsContribute.ts',
        editUrl: ({docPath}: {docPath: string}) => `${contributeEditBase}${docPath}`,
        showLastUpdateAuthor: true,
        showLastUpdateTime: true,
      },
    ],
    [
      '@docusaurus/plugin-content-docs',
      {
        id: 'releases',
        path: '../releases',
        routeBasePath: 'releases',
        sidebarPath: './sidebarsReleases.ts',
        editUrl: () => `${repoUrl}/edit/main/CHANGELOG.md`,
        showLastUpdateAuthor: false,
        showLastUpdateTime: true,
      },
    ],
    [
      '@docusaurus/plugin-client-redirects',
      {
        redirects: [
          {to: '/docs', from: ['/latest']},
          {to: '/docs/next', from: ['/dev']},
          {
            to: '/docs/get-started',
            from: ['/latest/user-guide'],
          },
          {
            to: '/docs/get-started/deployment-decision-guide',
            from: ['/latest/user-guide/deployment-decision-guide'],
          },
          {
            to: '/docs/security',
            from: ['/latest/security'],
          },
          {
            to: '/docs/architecture',
            from: ['/latest/architecture'],
          },
          {
            to: '/docs/reference/compatibility',
            from: ['/latest/reference/compatibility'],
          },
          {
            to: '/contribute',
            from: ['/latest/contributing'],
          },
          {
            to: '/docs/next/get-started',
            from: ['/dev/user-guide'],
          },
          {
            to: '/docs/next/security',
            from: ['/dev/security'],
          },
          {
            to: '/docs/next/architecture',
            from: ['/dev/architecture'],
          },
          {
            to: '/contribute',
            from: ['/dev/contributing'],
          },
          {
            to: '/docs/operate/backups',
            from: [
              '/docs/validated-deployments/runbooks/scheduled-backups-s3-compatible',
              '/docs/next/validated-deployments/runbooks/scheduled-backups-s3-compatible',
            ],
          },
          {
            to: '/docs/user-guide/openbaorestore/restore',
            from: [
              '/docs/validated-deployments/runbooks/restore-from-s3-compatible-snapshot',
              '/docs/next/validated-deployments/runbooks/restore-from-s3-compatible-snapshot',
            ],
          },
        ],
      },
    ],
    [
      '@cmfcmf/docusaurus-search-local',
      {
        indexDocs: true,
        indexBlog: false,
        indexDocSidebarParentCategories: 2,
        includeParentCategoriesInPageTitle: true,
        indexPages: true,
        language: 'en',
        maxSearchResults: 8,
      },
    ],
  ],

  themeConfig: {
    image: 'img/brand/repo_logo.png',
    colorMode: {
      respectPrefersColorScheme: true,
    },
    docs: {
      sidebar: {
        hideable: true,
      },
    },
    navbar: {
      title: 'OpenBao Operator',
      logo: {
        alt: 'OpenBao Operator',
        src: 'img/brand/logo.svg',
      },
      items: [
        {
          label: 'Docs',
          position: 'left',
          activeBaseRegex: '/(docs|releases)/',
          items: [
            {
              to: '/docs/get-started',
              label: 'Get Started',
            },
            {
              to: '/docs/configure',
              label: 'Configure',
            },
            {
              to: '/docs/operate',
              label: 'Operate',
            },
            {
              to: '/docs/validated-deployments',
              label: 'Validated Deployments',
            },
            {
              to: '/docs/reference',
              label: 'Reference',
            },
            {
              to: '/releases',
              label: 'Releases',
            },
          ],
        },
        {
          to: '/docs/security',
          label: 'Security',
          position: 'left',
        },
        {
          to: '/docs/architecture',
          label: 'Architecture',
          position: 'left',
        },
        {
          to: '/contribute',
          label: 'Contribute',
          position: 'left',
          activeBaseRegex: '/contribute/',
        },
        {
          type: 'docsVersionDropdown',
          position: 'right',
          docsPluginId: 'default',
          dropdownActiveClassDisabled: true,
        },
        {
          href: repoUrl,
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Operate',
          items: [
            {label: 'Get Started', to: '/docs/get-started'},
            {label: 'Install', to: '/docs/get-started/install'},
            {label: 'Production Checklist', to: '/docs/operate/production-checklist'},
          ],
        },
        {
          title: 'Reference',
          items: [
            {label: 'API', to: '/docs/reference/api'},
            {label: 'Compatibility', to: '/docs/reference/compatibility'},
            {label: 'Releases', to: '/releases'},
          ],
        },
        {
          title: 'Project',
          items: [
            {label: 'Repository', href: repoUrl},
            {label: 'Artifact Hub', href: 'https://artifacthub.io/packages/search?repo=openbao-operator'},
            {label: 'Edge Manifests', href: 'https://dc-tec.github.io/openbao-operator/edge/latest/install.yaml'},
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} OpenBao Operator contributors.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.nightOwl,
      additionalLanguages: ['bash', 'diff', 'go', 'json', 'yaml'],
    },
    mermaid: {
      theme: {light: 'neutral', dark: 'dark'},
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
