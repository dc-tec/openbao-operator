import React from 'react';
import clsx from 'clsx';
import {ArrowRight} from 'lucide-react';
import SiteLink from '@site/src/components/SiteLink';

type JourneyHeroAction = {
  docId?: string;
  label: string;
  to?: string;
  variant?: 'primary' | 'secondary' | 'outline';
};

type JourneyHeroProps = {
  children?: React.ReactNode;
  className?: string;
  eyebrow?: string;
  title: string;
  lede: string;
  actions?: JourneyHeroAction[];
};

const actionClasses: Record<NonNullable<JourneyHeroAction['variant']>, string> = {
  outline: 'button button--outline',
  primary: 'button button--primary',
  secondary: 'button button--secondary',
};

function JourneyHeroActionLink({
  docId,
  label,
  to,
  variant = 'primary',
}: JourneyHeroAction): React.JSX.Element {
  return (
    <SiteLink className={clsx(actionClasses[variant], 'journeyHero__action')} docId={docId} to={to}>
      {label}
      <ArrowRight size={16} />
    </SiteLink>
  );
}

export default function JourneyHero({
  children,
  className,
  eyebrow,
  title,
  lede,
  actions = [],
}: JourneyHeroProps): React.JSX.Element {
  return (
    <section
      className={clsx('journeyHero', className, {
        'journeyHero--withAside': Boolean(children),
      })}
    >
      <div className="journeyHero__content">
        {eyebrow ? <p className="journeyHero__eyebrow">{eyebrow}</p> : null}
        <h1>{title}</h1>
        <p className="journeyHero__lede">{lede}</p>
        {actions.length > 0 ? (
          <div className="journeyHero__actions">
            {actions.map((action) => (
              <JourneyHeroActionLink
                key={`${action.label}-${action.docId ?? action.to}`}
                {...action}
              />
            ))}
          </div>
        ) : null}
      </div>
      {children ? <aside className="journeyHero__aside">{children}</aside> : null}
    </section>
  );
}
