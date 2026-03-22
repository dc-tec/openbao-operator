import React from 'react';
import {ArrowRight} from 'lucide-react';
import SiteLink from '@site/src/components/SiteLink';

type NextAction = {
  description?: React.ReactNode;
  docId?: string;
  label: string;
  to?: string;
};

type NextActionsProps = {
  items: NextAction[];
  title?: string;
};

export default function NextActions({
  items,
  title = 'Next actions',
}: NextActionsProps): React.JSX.Element {
  return (
    <section className="nextActions">
      <p className="nextActions__title">{title}</p>
      <div className="nextActions__list">
        {items.map((item) => (
          <SiteLink
            key={`${item.label}-${item.docId ?? item.to}`}
            className="nextActions__item"
            docId={item.docId}
            to={item.to}
          >
            <span className="nextActions__label">
              {item.label}
              <ArrowRight size={16} />
            </span>
            {item.description ? (
              <span className="nextActions__description">{item.description}</span>
            ) : null}
          </SiteLink>
        ))}
      </div>
    </section>
  );
}
