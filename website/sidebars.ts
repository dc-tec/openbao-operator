import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  operatorDocs: [
    'index',
    {
      type: 'category',
      label: 'Get Started',
      link: {
        type: 'doc',
        id: 'user-guide/index',
      },
      items: [
        {
          type: 'doc',
          id: 'user-guide/operator/deployment-decision-guide',
          label: '1. Choose a deployment model',
        },
        {
          type: 'doc',
          id: 'user-guide/operator/installation',
          label: '2. Install the operator',
        },
        {
          type: 'doc',
          id: 'user-guide/openbaocluster/getting-started',
          label: '3. Create your first cluster',
        },
        {
          type: 'doc',
          id: 'user-guide/openbaocluster/next-steps',
          label: '4. Prepare for day 2',
        },
        {
          type: 'doc',
          id: 'user-guide/operator/single-tenant-mode',
          label: 'Single-tenant branch',
        },
      ],
    },
    {
      type: 'category',
      label: 'Identity & Tenancy',
      items: [
        'user-guide/operator/identity-and-access',
        'user-guide/operator/authn',
        'user-guide/operator/authz',
        'user-guide/openbaotenant/overview',
        'user-guide/openbaotenant/onboarding',
        'user-guide/openbaotenant/multi-tenancy',
      ],
    },
    {
      type: 'category',
      label: 'Configure',
      link: {
        type: 'doc',
        id: 'user-guide/openbaocluster/configuration/index',
      },
      items: [
        {
          type: 'doc',
          id: 'user-guide/openbaocluster/overview',
          label: 'Cluster overview',
        },
        'user-guide/openbaocluster/configuration/security-profiles',
        'user-guide/openbaocluster/configuration/self-init',
        'user-guide/openbaocluster/configuration/server',
        'user-guide/openbaocluster/configuration/external-access',
        'user-guide/openbaocluster/configuration/network',
        'user-guide/openbaocluster/configuration/air-gapped',
        'user-guide/openbaocluster/configuration/resources-storage',
        'user-guide/openbaocluster/configuration/gateway-api',
        'user-guide/openbaocluster/configuration/observability',
        'user-guide/openbaocluster/security-considerations',
      ],
    },
    {
      type: 'category',
      label: 'Operate',
      link: {
        type: 'doc',
        id: 'user-guide/openbaocluster/operations/index',
      },
      items: [
        {
          type: 'category',
          label: 'Routine Operations',
          items: [
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/operations/upgrades',
              label: 'Plan upgrades',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/operations/backups',
              label: 'Configure backups',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/operations/maintenance',
              label: 'Run maintenance',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/operations/pausing',
              label: 'Pause reconciliation',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/operations/deletion',
              label: 'Decommission a cluster',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/operations/troubleshooting',
              label: 'Troubleshoot the cluster',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/operations/production-checklist',
              label: 'Complete the production checklist',
            },
          ],
        },
        {
          type: 'category',
          label: 'Recovery & Restore',
          link: {
            type: 'doc',
            id: 'user-guide/openbaocluster/recovery/index',
          },
          items: [
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/recovery/safe-mode',
              label: 'Enter safe mode',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/recovery/no-leader',
              label: 'Recover from no leader',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/recovery/sealed-cluster',
              label: 'Recover a sealed cluster',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/recovery/failed-rollback',
              label: 'Recover from failed rollback',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaorestore/overview',
              label: 'Restore overview',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaorestore/restore',
              label: 'Restore from backup',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaorestore/recovery-restore-after-upgrade',
              label: 'Recover after upgrade restore',
            },
          ],
        },
      ],
    },
    {
      type: 'category',
      label: 'Validated Deployments',
      link: {
        type: 'doc',
        id: 'user-guide/validated-deployments/index',
      },
      items: [
        'user-guide/validated-deployments/architectures/index',
        'user-guide/validated-deployments/architectures/cloud/index',
        'user-guide/validated-deployments/architectures/cloud/amazon-eks-development-awskms-s3',
        'user-guide/validated-deployments/architectures/cloud/amazon-eks-hardened-awskms-acme',
        'user-guide/validated-deployments/architectures/local/index',
        'user-guide/validated-deployments/architectures/local/k3d-development-shared-edge-rustfs',
        'user-guide/validated-deployments/architectures/local/k3d-hardened-transit-external-tls',
        'user-guide/validated-deployments/architectures/local/k3d-hardened-transit-acme',
        'user-guide/validated-deployments/architectures/local/k3d-cross-cluster-dr-transit-rustfs',
        'user-guide/validated-deployments/recipes/index',
        'user-guide/validated-deployments/recipes/cloud/index',
        'user-guide/validated-deployments/recipes/cloud/amazon-eks-development-awskms-s3',
        'user-guide/validated-deployments/recipes/cloud/amazon-eks-hardened-awskms-acme',
        'user-guide/validated-deployments/recipes/local/index',
        'user-guide/validated-deployments/recipes/local/development-self-init-userpass',
        'user-guide/validated-deployments/recipes/local/hardened-transit-external-tls',
        'user-guide/validated-deployments/recipes/local/hardened-transit-acme-tls',
        'user-guide/validated-deployments/recipes/local/k3d-cross-cluster-dr-bootstrap',
        'user-guide/validated-deployments/runbooks/index',
        'user-guide/validated-deployments/runbooks/scheduled-backups-s3-compatible',
        'user-guide/validated-deployments/runbooks/restore-from-s3-compatible-snapshot',
        'user-guide/validated-deployments/runbooks/cross-cluster-dr-restore-rustfs',
      ],
    },
    {
      type: 'category',
      label: 'Security',
      link: {
        type: 'doc',
        id: 'security/index',
      },
      items: [
        'security/fundamentals/index',
        'security/fundamentals/threat-model',
        'security/fundamentals/profiles',
        'security/fundamentals/secrets-management',
        'security/infrastructure/index',
        'security/infrastructure/rbac',
        'security/infrastructure/admission-policies',
        'security/infrastructure/network-security',
        'security/workload/index',
        'security/workload/workload-security',
        'security/workload/tls',
        'security/workload/supply-chain',
        'security/multi-tenancy/index',
        'security/multi-tenancy/tenant-isolation',
      ],
    },
    {
      type: 'category',
      label: 'Architecture',
      link: {
        type: 'doc',
        id: 'architecture/index',
      },
      items: [
        'architecture/operator-invariants',
        'architecture/components',
        'architecture/cert-manager',
        'architecture/infra-manager',
        'architecture/init-manager',
        'architecture/upgrade-manager',
        'architecture/backup-manager',
        'architecture/restore-manager',
        'architecture/provisioner-manager',
        'architecture/operation-lifecycle',
        'architecture/lifecycle/index',
        'architecture/lifecycle/day0-provisioning',
        'architecture/lifecycle/day1-creation',
        'architecture/lifecycle/day2-operations',
        'architecture/lifecycle/dayN-backups',
      ],
    },
    {
      type: 'category',
      label: 'Reference',
      link: {
        type: 'doc',
        id: 'reference/index',
      },
      items: [
        'reference/api',
        'reference/compatibility',
        'reference/deprecation-policy',
        'reference/support-policy',
        'reference/operator-upgrade-compatibility',
        'reference/status-and-events',
        'reference/known-limitations',
      ],
    },
  ],
};

export default sidebars;
