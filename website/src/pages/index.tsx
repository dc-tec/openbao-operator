import React from 'react';
import Layout from '@theme/Layout';
import Link from '@docusaurus/Link';
import {ArrowRight} from 'lucide-react';
import NextActions from '@site/src/components/NextActions';
import RouteList from '@site/src/components/RouteList';
import styles from './index.module.css';

const primaryRoutes = [
  {
    eyebrow: '01',
    title: 'Get Started',
    description:
      'Choose a deployment model, install the operator, and create the first cluster with a clear day 2 handoff.',
    docId: 'user-guide/index',
  },
  {
    eyebrow: '02',
    title: 'Operate',
    description:
      'Plan upgrades, backups, maintenance, troubleshooting, and recovery around the production lifecycle.',
    docId: 'user-guide/openbaocluster/operations/index',
  },
  {
    eyebrow: '03',
    title: 'Security',
    description:
      'Review trust boundaries, admission controls, workload posture, and multi-tenant boundaries.',
    docId: 'security/index',
  },
];

const supportingRoutes = [
  {
    label: 'Open configure',
    description:
      'Set profiles, bootstrap, exposure, storage, and observability before the cluster becomes expensive to change.',
    docId: 'user-guide/openbaocluster/configuration/index',
  },
  {
    label: 'Validated deployments',
    description: 'Use tested architectures, recipes, and runbooks when you want a known-good starting point.',
    docId: 'user-guide/validated-deployments/index',
  },
  {
    label: 'Open architecture',
    description: 'Controller boundaries, lifecycle design, and the reasoning behind operator behavior.',
    docId: 'architecture/index',
  },
  {
    label: 'Open reference',
    description: 'Compatibility, API reference, support posture, and exact status and event semantics.',
    docId: 'reference/index',
  },
  {
    label: 'Read release notes',
    description: 'Versioned release context that stays aligned with the published docs and artifacts.',
    to: '/releases',
  },
  {
    label: 'Contributor docs',
    description: 'Development setup, standards, testing, CI, and release management guidance.',
    to: '/contribute',
  },
];

const lifecycleOverview = [
  {
    label: 'Decide',
    detail: 'Choose tenancy, trust boundaries, and installation ownership before you touch the cluster.',
  },
  {
    label: 'Install',
    detail: 'Render the operator with the right namespace, identity, and admission controls.',
  },
  {
    label: 'Operate',
    detail: 'Move quickly into upgrades, backups, maintenance, and production hardening.',
  },
  {
    label: 'Recover',
    detail: 'Use explicit runbooks for no-leader events, sealed clusters, and restore paths.',
  },
];

export default function Home(): React.JSX.Element {
  return (
    <Layout
      title="OpenBao Operator"
      description="OpenBao Operator documentation for secure lifecycle management on Kubernetes."
    >
      <main className={styles.page}>
        <section className={styles.hero}>
          <div className={styles.heroInner}>
            <div className={styles.heroCopy}>
              <p className={styles.kicker}>OpenBao Operator</p>
              <h1>Operate OpenBao like a platform, not a pile of YAML.</h1>
              <p className={styles.lede}>
                Install, configure, operate, secure, and recover OpenBao on Kubernetes with
                documentation written for platform teams running it as a real service.
              </p>
              <div className={styles.heroActions}>
                <Link className="button button--primary button--lg" to="/docs/get-started">
                  Start with Get Started
                </Link>
                <Link className="button button--secondary button--lg" to="/releases">
                  Release Notes
                </Link>
              </div>
              <ul className={styles.heroList}>
                <li>Install with the right tenancy, identity, and admission controls</li>
                <li>Move from first cluster to upgrades, backups, and production checks</li>
                <li>Use explicit runbooks when the cluster stops behaving normally</li>
              </ul>
            </div>

            <aside className={styles.overviewRail} aria-label="Operator lifecycle overview">
              <p className={styles.overviewKicker}>Operator lifecycle</p>
              <ol className={styles.overviewList}>
                {lifecycleOverview.map((step, index) => (
                  <li key={step.label} className={styles.overviewItem}>
                    <span className={styles.overviewMarker}>
                      <span className={styles.overviewDot} />
                    </span>
                    <div className={styles.overviewBody}>
                      <p className={styles.overviewStep}>{step.label}</p>
                      <p className={styles.overviewDetail}>{step.detail}</p>
                    </div>
                    <span className={styles.overviewIndex}>
                      {String(index + 1).padStart(2, '0')}
                    </span>
                  </li>
                ))}
              </ol>
              <p className={styles.overviewNote}>
                Start with Get Started for a new install. Use the other sections when the job is
                already operational or incident-driven.
              </p>
            </aside>
          </div>
        </section>

        <section className={styles.routeSection}>
          <div className={styles.sectionLead}>
            <p className={styles.sectionKicker}>Primary routes</p>
            <h2>Start with the task in front of you.</h2>
            <p>
              Use these sections to install the operator, run it in production, or review the
              platform security model without guessing where to begin.
            </p>
          </div>

          <RouteList items={primaryRoutes} />
        </section>

        <section className={styles.supportSection}>
          <div className={styles.sectionLead}>
            <p className={styles.sectionKicker}>Secondary destinations</p>
            <h2>Use reference pages when you need detail, not direction.</h2>
            <p>
              Configuration guides, validated deployments, reference pages, release notes, and
              contributor guidance stay close to the operational guides without taking over the main
              entry points.
            </p>
          </div>

          <NextActions items={supportingRoutes} title="Browse supporting sections" />
        </section>

        <section className={styles.finalSection}>
          <p className={styles.sectionKicker}>Start here</p>
          <h2>New install? Start with Get Started.</h2>
          <p>
            If the operator is already running, use Operate for routine work and recovery, or
            Security when you need to review trust boundaries and controls.
          </p>
          <div className={styles.finalActions}>
            <Link className="button button--primary button--lg" to="/docs/get-started">
              Open Docs
            </Link>
            <Link className={styles.inlineLink} to="/docs/security">
              Review security posture
              <ArrowRight size={16} />
            </Link>
          </div>
        </section>
      </main>
    </Layout>
  );
}
