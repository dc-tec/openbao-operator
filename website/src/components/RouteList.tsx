import React from 'react';
import clsx from 'clsx';
import {ArrowRight} from 'lucide-react';
import SiteLink from '@site/src/components/SiteLink';
import renderInlineCode from '@site/src/components/renderInlineCode';

type RouteListItem = {
  actionLabel?: string;
  description: React.ReactNode;
  docId?: string;
  eyebrow?: React.ReactNode;
  title: React.ReactNode;
  to?: string;
};

type RouteListProps = {
  className?: string;
  items: RouteListItem[];
  title?: React.ReactNode;
};

export default function RouteList({
  className,
  items,
  title,
}: RouteListProps): React.JSX.Element {
  return (
    <section className={clsx('routeListBlock', className)}>
      {title ? <p className="routeListBlock__title">{renderInlineCode(title)}</p> : null}
      <ol className="routeListBlock__list">
        {items.map((item, index) => (
          <li key={`route-${item.docId ?? item.to ?? index}`} className="routeListBlock__item">
            <span className="routeListBlock__eyebrow">
              {renderInlineCode(item.eyebrow ?? String(index + 1).padStart(2, '0'))}
            </span>
            <div className="routeListBlock__body">
              <h3>{renderInlineCode(item.title)}</h3>
              <p>{renderInlineCode(item.description)}</p>
            </div>
            <SiteLink
              className="routeListBlock__link"
              docId={item.docId}
              to={item.to}
            >
              {renderInlineCode(item.actionLabel ?? 'Open')}
              <ArrowRight size={16} />
            </SiteLink>
          </li>
        ))}
      </ol>
    </section>
  );
}
