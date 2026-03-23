import React from 'react';
import {ArrowRight} from 'lucide-react';
import SiteLink from '@site/src/components/SiteLink';
import renderInlineCode from '@site/src/components/renderInlineCode';

type NextAction = {
  description?: React.ReactNode;
  docId?: string;
  label: React.ReactNode;
  to?: string;
};

type NextActionsProps = {
  items: NextAction[];
  title?: React.ReactNode;
};

export default function NextActions({
  items,
  title = 'Next actions',
}: NextActionsProps): React.JSX.Element {
  return (
    <section className="nextActions">
      <p className="nextActions__title">{renderInlineCode(title)}</p>
      <div className="nextActions__list">
        {items.map((item, index) => (
          <SiteLink
            key={`next-action-${item.docId ?? item.to ?? index}`}
            className="nextActions__item"
            docId={item.docId}
            to={item.to}
          >
            <span className="nextActions__label">
              {renderInlineCode(item.label)}
              <ArrowRight size={16} />
            </span>
            {item.description ? (
              <span className="nextActions__description">{renderInlineCode(item.description)}</span>
            ) : null}
          </SiteLink>
        ))}
      </div>
    </section>
  );
}
