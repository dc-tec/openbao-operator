import React from 'react';
import clsx from 'clsx';
import {AlertTriangle, CheckCircle2} from 'lucide-react';

type ChecklistProps = {
  items: string[];
  title?: string;
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
      {title ? <p className="checklist__title">{title}</p> : null}
      <ul className="checklist__list">
        {items.map((item) => (
          <li key={item} className="checklist__item">
            <Icon className="checklist__icon" size={18} strokeWidth={2.1} />
            <span>{item}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}
