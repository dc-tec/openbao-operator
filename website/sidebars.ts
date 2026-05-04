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
          type: 'category',
          label: 'Core Path',
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
              id: 'user-guide/openbaotenant/onboarding',
              label: '3. Onboard the target namespace',
            },
            {
              type: 'doc',
              id: 'user-guide/service-claims/overview',
              label: '4. Choose the provisioning model',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/getting-started',
              label: '5. Create your first cluster',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/next-steps',
              label: '6. Prepare for day 2',
            },
          ],
        },
        {
          type: 'category',
          label: 'Supporting Decisions',
          items: [
            {
              type: 'doc',
              id: 'user-guide/operator/single-tenant-mode',
              label: 'Single-tenant mode',
            },
            {
              type: 'doc',
              id: 'user-guide/operator/identity-and-access',
              label: 'Operator identity and access',
            },
            {
              type: 'doc',
              id: 'user-guide/operator/authn',
              label: 'Operator authentication',
            },
            {
              type: 'doc',
              id: 'user-guide/operator/authz',
              label: 'Operator authorization',
            },
          ],
        },
      ],
    },
    {
      type: 'category',
      label: 'Service Claims',
      link: {
        type: 'doc',
        id: 'user-guide/service-claims/index',
      },
      items: [
        {
          type: 'doc',
          id: 'user-guide/service-claims/overview',
          label: 'Choose service claims',
        },
        {
          type: 'doc',
          id: 'user-guide/service-claims/service-catalog',
          label: 'Understand catalog objects',
        },
        {
          type: 'doc',
          id: 'user-guide/service-claims/support-matrix',
          label: 'Check supported shapes',
        },
        {
          type: 'doc',
          id: 'user-guide/service-claims/publish-service-catalog',
          label: 'Publish a minimum catalog',
        },
        {
          type: 'doc',
          id: 'user-guide/service-claims/publish-production-catalog',
          label: 'Publish a production catalog',
        },
        {
          type: 'doc',
          id: 'user-guide/service-claims/getting-started',
          label: 'Request a service',
        },
        {
          type: 'doc',
          id: 'user-guide/service-claims/day-2-workflows',
          label: 'Operate claim services',
        },
        {
          type: 'doc',
          id: 'user-guide/service-claims/exposure',
          label: 'Plan exposure',
        },
        {
          type: 'doc',
          id: 'user-guide/service-claims/bootstrap-dependencies',
          label: 'Plan bootstrap dependencies',
        },
        {
          type: 'doc',
          id: 'user-guide/service-claims/troubleshooting',
          label: 'Troubleshoot claim services',
        },
        {
          type: 'doc',
          id: 'user-guide/service-claims/unsupported-workflows',
          label: 'Unsupported workflows',
        },
      ],
    },
    {
      type: 'category',
      label: 'Tenant Onboarding',
      items: [
        {
          type: 'doc',
          id: 'user-guide/openbaotenant/overview',
          label: 'Tenancy & Governance',
        },
        {
          type: 'doc',
          id: 'user-guide/openbaotenant/multi-tenancy',
          label: 'Multi-tenant security',
        },
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
          type: 'category',
          label: 'Read This First',
          items: [
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/overview',
              label: 'Cluster overview',
            },
          ],
        },
        {
          type: 'category',
          label: 'Cluster Baseline',
          items: [
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/configuration/security-profiles',
              label: 'Security profiles',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/configuration/self-init',
              label: 'Self-initialization',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/configuration/unseal',
              label: 'Unseal configuration',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/configuration/server',
              label: 'Server configuration',
            },
          ],
        },
        {
          type: 'category',
          label: 'Service Boundary',
          items: [
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/configuration/external-access',
              label: 'External access',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/configuration/network',
              label: 'Network configuration',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/configuration/gateway-api',
              label: 'Gateway API support',
            },
          ],
        },
        {
          type: 'category',
          label: 'Read Scaling',
          items: [
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/configuration/read-replicas',
              label: 'Read replicas',
            },
          ],
        },
        {
          type: 'category',
          label: 'Platform Readiness',
          items: [
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/configuration/resources-storage',
              label: 'Resources and storage',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/configuration/observability',
              label: 'Observability',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/configuration/air-gapped',
              label: 'Air-gapped and private registries',
            },
          ],
        },
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
          label: 'Reliability & Change',
          items: [
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/operations/production-checklist',
              label: 'Production checklist',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/operations/backups',
              label: 'Configure backups',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/operations/upgrades',
              label: 'Plan upgrades',
            },
          ],
        },
        {
          type: 'category',
          label: 'Cluster Controls',
          items: [
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/operations/maintenance',
              label: 'Run planned maintenance',
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
          ],
        },
        {
          type: 'category',
          label: 'Troubleshooting & Recovery',
          items: [
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/operations/troubleshooting',
              label: 'Troubleshoot the cluster',
            },
            {
              type: 'doc',
              id: 'user-guide/openbaocluster/recovery/index',
              label: 'Recovery & Restore',
            },
            {
              type: 'category',
              label: 'Incident Recovery',
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
              ],
            },
            {
              type: 'category',
              label: 'Restore from Backup',
              items: [
                {
                  type: 'doc',
                  id: 'user-guide/openbaorestore/restore',
                  label: 'Run a restore',
                },
                {
                  type: 'doc',
                  id: 'user-guide/openbaorestore/overview',
                  label: 'Restore overview',
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
        {
          type: 'category',
          label: 'Cloud Baselines',
          items: [
            {
              type: 'doc',
              id: 'user-guide/validated-deployments/architectures/cloud/index',
              label: 'Cloud overview',
            },
            {
              type: 'category',
              label: 'EKS Development',
              items: [
                {
                  type: 'doc',
                  id: 'user-guide/validated-deployments/architectures/cloud/amazon-eks-development-awskms-s3',
                  label: 'Reference architecture',
                },
                {
                  type: 'doc',
                  id: 'user-guide/validated-deployments/recipes/cloud/amazon-eks-development-awskms-s3',
                  label: 'Deployment recipe',
                },
              ],
            },
            {
              type: 'category',
              label: 'EKS Hardened',
              items: [
                {
                  type: 'doc',
                  id: 'user-guide/validated-deployments/architectures/cloud/amazon-eks-hardened-awskms-acme',
                  label: 'Reference architecture',
                },
                {
                  type: 'doc',
                  id: 'user-guide/validated-deployments/recipes/cloud/amazon-eks-hardened-awskms-acme',
                  label: 'Deployment recipe',
                },
              ],
            },
          ],
        },
        {
          type: 'category',
          label: 'Local Baselines',
          items: [
            {
              type: 'doc',
              id: 'user-guide/validated-deployments/architectures/local/index',
              label: 'Local overview',
            },
            {
              type: 'category',
              label: 'k3d Development',
              items: [
                {
                  type: 'doc',
                  id: 'user-guide/validated-deployments/architectures/local/k3d-development-shared-edge-rustfs',
                  label: 'Reference architecture',
                },
                {
                  type: 'doc',
                  id: 'user-guide/validated-deployments/recipes/local/development-self-init-userpass',
                  label: 'Deployment recipe',
                },
              ],
            },
            {
              type: 'category',
              label: 'k3d Hardened / External TLS',
              items: [
                {
                  type: 'doc',
                  id: 'user-guide/validated-deployments/architectures/local/k3d-hardened-transit-external-tls',
                  label: 'Reference architecture',
                },
                {
                  type: 'doc',
                  id: 'user-guide/validated-deployments/recipes/local/hardened-transit-external-tls',
                  label: 'Deployment recipe',
                },
              ],
            },
            {
              type: 'category',
              label: 'k3d Hardened / ACME',
              items: [
                {
                  type: 'doc',
                  id: 'user-guide/validated-deployments/architectures/local/k3d-hardened-transit-acme',
                  label: 'Reference architecture',
                },
                {
                  type: 'doc',
                  id: 'user-guide/validated-deployments/recipes/local/hardened-transit-acme-tls',
                  label: 'Deployment recipe',
                },
              ],
            },
            {
              type: 'category',
              label: 'k3d Cross-Cluster DR',
              items: [
                {
                  type: 'doc',
                  id: 'user-guide/validated-deployments/architectures/local/k3d-cross-cluster-dr-transit-rustfs',
                  label: 'Reference architecture',
                },
                {
                  type: 'doc',
                  id: 'user-guide/validated-deployments/recipes/local/k3d-cross-cluster-dr-bootstrap',
                  label: 'Bootstrap recipe',
                },
                {
                  type: 'doc',
                  id: 'user-guide/validated-deployments/runbooks/cross-cluster-dr-restore-rustfs',
                  label: 'DR restore runbook',
                },
              ],
            },
          ],
        },
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
        {
          type: 'category',
          label: 'Security Model',
          items: [
            {
              type: 'doc',
              id: 'security/fundamentals/index',
              label: 'Security model overview',
            },
            {
              type: 'doc',
              id: 'security/fundamentals/threat-model',
              label: 'Threat model',
            },
            {
              type: 'doc',
              id: 'security/fundamentals/profiles',
              label: 'Production posture',
            },
            {
              type: 'doc',
              id: 'security/fundamentals/secrets-management',
              label: 'Secrets and trust material',
            },
          ],
        },
        {
          type: 'category',
          label: 'Platform Controls',
          items: [
            {
              type: 'doc',
              id: 'security/infrastructure/index',
              label: 'Platform controls overview',
            },
            {
              type: 'doc',
              id: 'security/infrastructure/rbac',
              label: 'RBAC architecture',
            },
            {
              type: 'doc',
              id: 'security/infrastructure/admission-policies',
              label: 'Admission policies',
            },
            {
              type: 'doc',
              id: 'security/infrastructure/network-security',
              label: 'Network security',
            },
          ],
        },
        {
          type: 'category',
          label: 'Workload Protections',
          items: [
            {
              type: 'doc',
              id: 'security/workload/index',
              label: 'Workload protections overview',
            },
            {
              type: 'doc',
              id: 'security/workload/workload-security',
              label: 'Pod and runtime security',
            },
            {
              type: 'doc',
              id: 'security/workload/tls',
              label: 'TLS and identity',
            },
            {
              type: 'doc',
              id: 'security/workload/supply-chain',
              label: 'Supply-chain verification',
            },
          ],
        },
        {
          type: 'category',
          label: 'Tenant Isolation',
          items: [
            {
              type: 'doc',
              id: 'security/multi-tenancy/index',
              label: 'Tenant isolation overview',
            },
            {
              type: 'doc',
              id: 'security/multi-tenancy/tenant-isolation',
              label: 'Isolation model',
            },
          ],
        },
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
        {
          type: 'category',
          label: 'Read This First',
          items: [
            {
              type: 'doc',
              id: 'architecture/operator-invariants',
              label: 'Operator invariants',
            },
            {
              type: 'doc',
              id: 'architecture/components',
              label: 'Component design',
            },
            {
              type: 'doc',
              id: 'architecture/service-claims',
              label: 'Service claims',
            },
            {
              type: 'doc',
              id: 'architecture/service-claims-contract-pipeline',
              label: 'Service-claim contract pipeline',
            },
            {
              type: 'doc',
              id: 'architecture/service-claims-boundaries',
              label: 'Service-claim boundaries',
            },
            {
              type: 'doc',
              id: 'architecture/service-claims-extension-guide',
              label: 'Extend service claims',
            },
            {
              type: 'doc',
              id: 'architecture/lifecycle/index',
              label: 'Lifecycle architecture',
            },
          ],
        },
        {
          type: 'category',
          label: 'Workload Managers',
          items: [
            {
              type: 'doc',
              id: 'architecture/workload-managers',
              label: 'Workload managers',
            },
            {
              type: 'doc',
              id: 'architecture/cert-manager',
              label: 'Cert manager',
            },
            {
              type: 'doc',
              id: 'architecture/init-manager',
              label: 'Init manager',
            },
          ],
        },
        {
          type: 'category',
          label: 'Operations Managers',
          items: [
            {
              type: 'doc',
              id: 'architecture/upgrade-manager',
              label: 'Upgrade manager',
            },
            {
              type: 'doc',
              id: 'architecture/backup-manager',
              label: 'Backup manager',
            },
            {
              type: 'doc',
              id: 'architecture/restore-manager',
              label: 'Restore manager',
            },
          ],
        },
        {
          type: 'category',
          label: 'Provisioning',
          items: [
            {
              type: 'doc',
              id: 'architecture/provisioner-manager',
              label: 'Provisioner manager',
            },
          ],
        },
        {
          type: 'category',
          label: 'Supporting Services',
          items: [
            {
              type: 'doc',
              id: 'architecture/operation-lifecycle',
              label: 'Operation lifecycle coordination',
            },
          ],
        },
        {
          type: 'category',
          label: 'Lifecycle Flows',
          items: [
            {
              type: 'doc',
              id: 'architecture/lifecycle/day0-provisioning',
              label: 'Day 0 provisioning',
            },
            {
              type: 'doc',
              id: 'architecture/lifecycle/day1-creation',
              label: 'Day 1 creation',
            },
            {
              type: 'doc',
              id: 'architecture/lifecycle/day2-operations',
              label: 'Day 2 operations',
            },
            {
              type: 'doc',
              id: 'architecture/lifecycle/dayN-backups',
              label: 'Day N backups and restore',
            },
          ],
        },
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
        {
          type: 'category',
          label: 'Quick Checks',
          items: [
            {
              type: 'doc',
              id: 'reference/compatibility',
              label: 'Compatibility matrix',
            },
            {
              type: 'doc',
              id: 'reference/operator-upgrade-compatibility',
              label: 'Upgrade compatibility',
            },
            {
              type: 'doc',
              id: 'reference/status-and-events',
              label: 'Status conditions and events',
            },
          ],
        },
        {
          type: 'category',
          label: 'API Surface',
          items: [
            {
              type: 'doc',
              id: 'reference/api',
              label: 'API reference',
            },
          ],
        },
        {
          type: 'category',
          label: 'Lifecycle & Support Contract',
          items: [
            {
              type: 'doc',
              id: 'reference/support-policy',
              label: 'Support policy',
            },
            {
              type: 'doc',
              id: 'reference/deprecation-policy',
              label: 'Deprecation policy',
            },
          ],
        },
        {
          type: 'category',
          label: 'Constraints & Caveats',
          items: [
            {
              type: 'doc',
              id: 'reference/known-limitations',
              label: 'Known limitations',
            },
          ],
        },
      ],
    },
    {
      type: 'category',
      label: 'Design',
      link: {
        type: 'doc',
        id: 'design/index',
      },
      items: [
        {
          type: 'doc',
          id: 'design/claims-and-service-offerings',
          label: 'Claims and Service Offerings',
        },
      ],
    },
    'roadmap',
  ],
};

export default sidebars;
