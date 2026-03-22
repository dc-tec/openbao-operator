import React from 'react';
import clsx from 'clsx';
import {ArrowRight, Compass, ShieldAlert, Sparkles} from 'lucide-react';
import SiteLink from '@site/src/components/SiteLink';

type OutcomeAction = {
  docId?: string;
  label: string;
  to?: string;
};

type OutcomePanelProps = {
  children: React.ReactNode;
  title: string;
  tone?: 'info' | 'success' | 'warning';
  actions?: OutcomeAction[];
};

const toneMeta = {
  info: {
    eyebrow: 'Keep moving',
    icon: Compass,
  },
  success: {
    eyebrow: 'Ready to move on',
    icon: Sparkles,
  },
  warning: {
    eyebrow: 'Do not skip this',
    icon: ShieldAlert,
  },
} as const;

function OutcomeActionLink({
  docId,
  label,
  to,
}: OutcomeAction): React.JSX.Element {
  return (
    <SiteLink className="outcomePanel__action" docId={docId} to={to}>
      {label}
      <ArrowRight size={16} />
    </SiteLink>
  );
}

export default function OutcomePanel({
  children,
  title,
  tone = 'info',
  actions = [],
}: OutcomePanelProps): React.JSX.Element {
  const meta = toneMeta[tone];
  const Icon = meta.icon;

  return (
    <section className={clsx('outcomePanel', `outcomePanel--${tone}`)}>
      <div className="outcomePanel__header">
        <span className="outcomePanel__icon">
          <Icon size={18} strokeWidth={2.1} />
        </span>
        <div>
          <p className="outcomePanel__eyebrow">{meta.eyebrow}</p>
          <p className="outcomePanel__title">{title}</p>
        </div>
      </div>
      <div className="outcomePanel__body">{children}</div>
      {actions.length > 0 ? (
        <div className="outcomePanel__actions">
          {actions.map((action) => (
            <OutcomeActionLink
              key={`${action.label}-${action.docId ?? action.to}`}
              {...action}
            />
          ))}
        </div>
      ) : null}
    </section>
  );
}
