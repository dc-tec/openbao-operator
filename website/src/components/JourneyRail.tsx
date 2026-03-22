import React from 'react';
import clsx from 'clsx';
import SiteLink from '@site/src/components/SiteLink';

type JourneyRailItem = {
  description: string;
  docId?: string;
  label: string;
  to?: string;
};

type JourneyRailProps = {
  current?: number;
  items: JourneyRailItem[];
  title?: string;
};

export default function JourneyRail({
  current,
  items,
  title = 'Journey',
}: JourneyRailProps): React.JSX.Element {
  return (
    <nav aria-label={title} className="journeyRail">
      <div className="journeyRail__header">
        <p className="journeyRail__eyebrow">Journey</p>
        <p className="journeyRail__title">{title}</p>
      </div>
      <ol className="journeyRail__list">
        {items.map((item, index) => {
          const step = index + 1;
          const state =
            current === undefined
              ? 'default'
              : step < current
                ? 'complete'
                : step === current
                  ? 'current'
                  : 'upcoming';

          return (
            <li
              key={`${item.label}-${step}`}
              className={clsx('journeyRail__item', `journeyRail__item--${state}`)}
            >
              <SiteLink
                aria-current={state === 'current' ? 'step' : undefined}
                className="journeyRail__link"
                docId={item.docId}
                to={item.to}
              >
                <span className="journeyRail__index">
                  {String(step).padStart(2, '0')}
                </span>
                <span className="journeyRail__copy">
                  <span className="journeyRail__label">{item.label}</span>
                  <span className="journeyRail__description">{item.description}</span>
                </span>
              </SiteLink>
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
