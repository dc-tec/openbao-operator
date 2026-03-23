import React from 'react';
import Layout from '@theme/Layout';
import Link from '@docusaurus/Link';
import {ArrowRight} from 'lucide-react';
import RouteList from '@site/src/components/RouteList';
import SiteLink from '@site/src/components/SiteLink';
import styles from './index.module.css';

const primaryRoutes = [
  {
    eyebrow: '01',
    title: 'Get Started',
    description:
      'Choose a deployment model, install the operator, onboard the target namespace, and create the first cluster.',
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

const planningRoutes = [
  {
    title: 'Configure',
    description:
      'Set profiles, bootstrap, exposure, storage, and observability before the cluster becomes expensive to change.',
    docId: 'user-guide/openbaocluster/configuration/index',
  },
  {
    title: 'Validated Deployments',
    description:
      'Use tested baselines, recipes, and DR lanes when you want a known-good deployment path to reproduce.',
    docId: 'user-guide/validated-deployments/index',
  },
];

const supportingRoutes = [
  {
    label: 'Architecture',
    description: 'Controller boundaries, lifecycle design, and the reasoning behind operator behavior.',
    docId: 'architecture/index',
  },
  {
    label: 'Reference',
    description: 'Compatibility, API reference, support posture, and exact status and event semantics.',
    docId: 'reference/index',
  },
  {
    label: 'Releases',
    description: 'Versioned release context that stays aligned with the published docs and artifacts.',
    to: '/releases',
  },
  {
    label: 'Contribute',
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
    label: 'Configure',
    detail: 'Shape exposure, storage, observability, and workload posture before production traffic arrives.',
  },
  {
    label: 'Operate',
    detail: 'Move into upgrades, backups, maintenance, troubleshooting, and production hardening.',
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
            </aside>
          </div>
        </section>

        <section className={styles.routeSection}>
          <div className={styles.sectionLead}>
            <p className={styles.sectionKicker}>Primary routes</p>
            <h2>Choose the route that matches the work.</h2>
            <p>
              These three sections cover first install, day 2 operations, and security review
              without forcing readers through the whole manual.
            </p>
          </div>

          <RouteList items={primaryRoutes} />
        </section>

        <section className={styles.planningSection}>
          <div className={styles.sectionLead}>
            <p className={styles.sectionKicker}>Deployment planning</p>
            <h2>Set the cluster shape before go-live.</h2>
            <p>
              Use Configure to make the baseline service decisions. Use Validated Deployments when
              you want a tested lane to reproduce instead of assembling that baseline yourself.
            </p>
          </div>

          <div className={styles.planningGrid}>
            {planningRoutes.map((route) => (
              <SiteLink
                key={route.title}
                className={styles.planningLink}
                docId={route.docId}
              >
                <span className={styles.planningLabel}>
                  {route.title}
                  <ArrowRight size={16} />
                </span>
                <span className={styles.planningDescription}>{route.description}</span>
              </SiteLink>
            ))}
          </div>
        </section>

        <section className={styles.supportSection}>
          <div className={styles.sectionLead}>
            <p className={styles.sectionKicker}>Secondary destinations</p>
            <h2>Keep architecture, reference, releases, and contributor guidance close.</h2>
            <p>
              Use these sections when the job is understanding behavior, checking exact contracts,
              reviewing release changes, or working on the project itself.
            </p>
          </div>

          <div className={styles.supportList} role="list" aria-label="Supporting sections">
            {supportingRoutes.map((route) => (
              <SiteLink
                key={route.label}
                className={styles.supportLink}
                docId={route.docId}
                to={route.to}
              >
                <span className={styles.supportLabel}>
                  {route.label}
                  <ArrowRight size={15} />
                </span>
                <span className={styles.supportDescription}>{route.description}</span>
              </SiteLink>
            ))}
          </div>
        </section>

        <section className={styles.finalSection}>
          <p className={styles.sectionKicker}>Quick handoff</p>
          <p className={styles.finalSummary}>
            New install? Start with Get Started. Day 2 work belongs in Operate. Exact
            compatibility, status, and policy questions belong in Reference.
          </p>
          <div className={styles.finalActions}>
            <Link className={styles.inlineLink} to="/docs/get-started">
              Get Started
              <ArrowRight size={16} />
            </Link>
            <Link className={styles.inlineLink} to="/docs/operate">
              Open Operate
              <ArrowRight size={16} />
            </Link>
            <Link className={styles.inlineLink} to="/docs/reference">
              Open Reference
              <ArrowRight size={16} />
            </Link>
          </div>
        </section>
      </main>
    </Layout>
  );
}
