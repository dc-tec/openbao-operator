import React from 'react';
import clsx from 'clsx';
import SiteLink from '@site/src/components/SiteLink';
import renderInlineCode from '@site/src/components/renderInlineCode';

type JourneyStepItem = {
  docId?: string;
  label: React.ReactNode;
  description: React.ReactNode;
  to?: string;
};

type JourneyStepsProps = {
  current: number;
  items: JourneyStepItem[];
  title?: React.ReactNode;
};

function JourneyStepsItem({
  description,
  docId,
  isCurrent,
  label,
  stepNumber,
  to,
}: JourneyStepItem & {
  isCurrent: boolean;
  stepNumber: number;
}): React.JSX.Element {
  const content = (
    <>
      <span className="journeySteps__index">{String(stepNumber).padStart(2, '0')}</span>
      <span className="journeySteps__copy">
        <span className="journeySteps__label">{renderInlineCode(label)}</span>
        <span className="journeySteps__description">{renderInlineCode(description)}</span>
      </span>
    </>
  );

  if (docId || to) {
    return (
      <SiteLink
        aria-current={isCurrent ? 'step' : undefined}
        className="journeySteps__link"
        docId={docId}
        to={to}
      >
        {content}
      </SiteLink>
    );
  }

  return <div className="journeySteps__link">{content}</div>;
}

export default function JourneySteps({
  current,
  items,
  title = 'Journey map',
}: JourneyStepsProps): React.JSX.Element {
  return (
    <nav aria-label={typeof title === 'string' ? title : 'Journey map'} className="journeySteps">
      <div className="journeySteps__header">
        <p className="journeySteps__eyebrow">Journey map</p>
        <p className="journeySteps__title">{renderInlineCode(title)}</p>
      </div>
      <ol className="journeySteps__list">
        {items.map((item, index) => {
          const stepNumber = index + 1;
          const state =
            stepNumber < current
              ? 'complete'
              : stepNumber === current
                ? 'current'
                : 'upcoming';
          return (
            <li
              key={`journey-step-${item.docId ?? item.to ?? stepNumber}`}
              className={clsx('journeySteps__step', `journeySteps__step--${state}`)}
            >
              <JourneyStepsItem
                {...item}
                isCurrent={state === 'current'}
                stepNumber={stepNumber}
              />
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
