import React from 'react';
import clsx from 'clsx';
import {ChevronDown, HelpCircle, Info} from 'lucide-react';

type ExpandableCalloutProps = {
  children: React.ReactNode;
  title?: string;
  type?: string;
};

export default function ExpandableCallout({
  children,
  title,
  type = 'note',
}: ExpandableCalloutProps): React.JSX.Element {
  return (
    <details className={clsx('expandableCallout', 'callout', `callout--${type}`)}>
      <summary>
        {type === 'question' ? <HelpCircle size={18} /> : <Info size={18} />}
        <span className="calloutTitle">{title ?? type}</span>
        <ChevronDown size={16} style={{marginLeft: 'auto'}} />
      </summary>
      <div className="expandableBody">{children}</div>
    </details>
  );
}
