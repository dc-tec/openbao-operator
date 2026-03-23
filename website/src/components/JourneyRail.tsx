import React from 'react';
import clsx from 'clsx';
import SiteLink from '@site/src/components/SiteLink';
import renderInlineCode from '@site/src/components/renderInlineCode';

type JourneyRailItem = {
  description: React.ReactNode;
  docId?: string;
  label: React.ReactNode;
  to?: string;
};

type JourneyRailProps = {
  current?: number;
  items: JourneyRailItem[];
  title?: React.ReactNode;
};

export default function JourneyRail({
  current,
  items,
  title = 'Journey',
}: JourneyRailProps): React.JSX.Element {
  return (
    <nav aria-label={typeof title === 'string' ? title : 'Journey'} className="journeyRail">
      <div className="journeyRail__header">
        <p className="journeyRail__eyebrow">Journey</p>
        <p className="journeyRail__title">{renderInlineCode(title)}</p>
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
              key={`journey-rail-${item.docId ?? item.to ?? step}`}
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
                  <span className="journeyRail__label">{renderInlineCode(item.label)}</span>
                  <span className="journeyRail__description">{renderInlineCode(item.description)}</span>
                </span>
              </SiteLink>
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
