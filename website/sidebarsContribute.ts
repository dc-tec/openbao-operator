import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  contributeDocs: [
    {
      type: 'doc',
      id: 'index',
      label: 'Contribute',
    },
    {
      type: 'category',
      label: 'Start Here',
      link: {
        type: 'doc',
        id: 'getting-started/index',
      },
      items: [
        {
          type: 'doc',
          id: 'getting-started/development',
          label: 'Set Up Your Environment',
        },
      ],
    },
    {
      type: 'category',
      label: 'Build & Change',
      link: {
        type: 'doc',
        id: 'standards/index',
      },
      items: [
        {
          type: 'doc',
          id: 'standards/project-conventions',
          label: 'Project Conventions',
        },
        {
          type: 'doc',
          id: 'standards/go-style',
          label: 'Go Style Guide',
        },
        {
          type: 'doc',
          id: 'standards/kubernetes-patterns',
          label: 'Kubernetes Operator Patterns',
        },
        {
          type: 'doc',
          id: 'standards/error-handling',
          label: 'Error Handling',
        },
        {
          type: 'doc',
          id: 'standards/generated-artifacts',
          label: 'Generated Artifacts',
        },
        {
          type: 'doc',
          id: 'standards/security-practices',
          label: 'Security Practices',
        },
        {
          type: 'doc',
          id: 'standards/conventional-commits',
          label: 'Conventional Commits',
        },
        {
          type: 'doc',
          id: 'docs-style-guide',
          label: 'Documentation Style Guide',
        },
      ],
    },
    {
      type: 'category',
      label: 'Validate & Ship',
      link: {
        type: 'doc',
        id: 'validate-and-ship',
      },
      items: [
        {
          type: 'doc',
          id: 'testing',
          label: 'Testing Strategy',
        },
        {
          type: 'doc',
          id: 'ci',
          label: 'Continuous Integration',
        },
        {
          type: 'doc',
          id: 'release-management',
          label: 'Release Management',
        },
        {
          type: 'doc',
          id: 'distribution',
          label: 'Distribution',
        },
      ],
    },
    {
      type: 'category',
      label: 'Project Governance',
      link: {
        type: 'doc',
        id: 'project-governance',
      },
      items: [
        {
          type: 'doc',
          id: 'sdlc',
          label: 'Software Development Lifecycle',
        },
        {
          type: 'doc',
          id: 'supply-chain-security',
          label: 'Supply Chain Security',
        },
        {
          type: 'doc',
          id: 'dependency-licenses',
          label: 'Dependency License Policy',
        },
      ],
    },
  ],
};

export default sidebars;
