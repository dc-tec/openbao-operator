import React from 'react';
import clsx from 'clsx';
import {ArrowRight} from 'lucide-react';
import SiteLink from '@site/src/components/SiteLink';
import renderInlineCode from '@site/src/components/renderInlineCode';

type PageHeroAction = {
  docId?: string;
  label: string;
  to?: string;
  variant?: 'primary' | 'secondary' | 'outline';
};

type PageHeroProps = {
  actions?: PageHeroAction[];
  children?: React.ReactNode;
  className?: string;
  eyebrow?: React.ReactNode;
  lede: React.ReactNode;
  title: React.ReactNode;
};

const actionClasses: Record<NonNullable<PageHeroAction['variant']>, string> = {
  outline: 'button button--outline',
  primary: 'button button--primary',
  secondary: 'button button--secondary',
};

export default function PageHero({
  actions = [],
  children,
  className,
  eyebrow,
  lede,
  title,
}: PageHeroProps): React.JSX.Element {
  return (
    <section
      className={clsx(
        'pageHero',
        className,
        {'pageHero--withAside': Boolean(children)},
      )}>
      <div className="pageHero__content">
        {eyebrow ? <p className="pageHero__eyebrow">{renderInlineCode(eyebrow)}</p> : null}
        <h1>{renderInlineCode(title)}</h1>
        <p className="pageHero__lede">{renderInlineCode(lede)}</p>
        {actions.length > 0 ? (
          <div className="pageHero__actions">
            {actions.map((action) => (
              <SiteLink
                key={`${action.label}-${action.docId ?? action.to}`}
                className={clsx(actionClasses[action.variant ?? 'primary'], 'pageHero__action')}
                docId={action.docId}
                to={action.to}
              >
                {renderInlineCode(action.label)}
                <ArrowRight size={16} />
              </SiteLink>
            ))}
          </div>
        ) : null}
      </div>
      {children ? <aside className="pageHero__aside">{children}</aside> : null}
    </section>
  );
}
