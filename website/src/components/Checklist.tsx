import React from 'react';
import clsx from 'clsx';
import {AlertTriangle, CheckCircle2} from 'lucide-react';
import renderInlineCode from '@site/src/components/renderInlineCode';

type ChecklistProps = {
  items: React.ReactNode[];
  title?: React.ReactNode;
  tone?: 'neutral' | 'success' | 'warning';
};

export default function Checklist({
  items,
  title,
  tone = 'neutral',
}: ChecklistProps): React.JSX.Element {
  const Icon = tone === 'warning' ? AlertTriangle : CheckCircle2;

  return (
    <section className={clsx('checklist', `checklist--${tone}`)}>
      {title ? <p className="checklist__title">{renderInlineCode(title)}</p> : null}
      <ul className="checklist__list">
        {items.map((item, index) => (
          <li key={`checklist-item-${index}`} className="checklist__item">
            <Icon className="checklist__icon" size={18} strokeWidth={2.1} />
            <span>{renderInlineCode(item)}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}
