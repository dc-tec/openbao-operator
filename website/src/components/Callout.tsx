import React from 'react';
import clsx from 'clsx';
import {AlertTriangle, CheckCircle2, HelpCircle, Info, ShieldAlert, XCircle} from 'lucide-react';

const icons = {
  abstract: Info,
  danger: ShieldAlert,
  example: Info,
  failure: XCircle,
  important: AlertTriangle,
  info: Info,
  note: Info,
  question: HelpCircle,
  success: CheckCircle2,
  tip: CheckCircle2,
  warning: AlertTriangle,
} as const;

type CalloutProps = {
  children: React.ReactNode;
  title?: string;
  type?: keyof typeof icons | string;
};

export default function Callout({
  children,
  title,
  type = 'note',
}: CalloutProps): React.JSX.Element {
  const Icon = icons[type as keyof typeof icons] ?? Info;

  return (
    <div className={clsx('callout', `callout--${type}`)}>
      <div className="calloutHeader">
        <Icon size={18} strokeWidth={2.2} />
        <span className="calloutTitle">{title ?? type}</span>
      </div>
      <div className="calloutBody">{children}</div>
    </div>
  );
}
