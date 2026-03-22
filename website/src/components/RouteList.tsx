import React from 'react';
import clsx from 'clsx';
import {ArrowRight} from 'lucide-react';
import SiteLink from '@site/src/components/SiteLink';

type RouteListItem = {
  actionLabel?: string;
  description: React.ReactNode;
  docId?: string;
  eyebrow?: string;
  title: string;
  to?: string;
};

type RouteListProps = {
  className?: string;
  items: RouteListItem[];
  title?: string;
};

export default function RouteList({
  className,
  items,
  title,
}: RouteListProps): React.JSX.Element {
  return (
    <section className={clsx('routeListBlock', className)}>
      {title ? <p className="routeListBlock__title">{title}</p> : null}
      <ol className="routeListBlock__list">
        {items.map((item, index) => (
          <li key={`${item.title}-${index}`} className="routeListBlock__item">
            <span className="routeListBlock__eyebrow">
              {item.eyebrow ?? String(index + 1).padStart(2, '0')}
            </span>
            <div className="routeListBlock__body">
              <h3>{item.title}</h3>
              <p>{item.description}</p>
            </div>
            <SiteLink
              className="routeListBlock__link"
              docId={item.docId}
              to={item.to}
            >
              {item.actionLabel ?? 'Open'}
              <ArrowRight size={16} />
            </SiteLink>
          </li>
        ))}
      </ol>
    </section>
  );
}
