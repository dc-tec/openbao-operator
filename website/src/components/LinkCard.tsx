import React from 'react';
import clsx from 'clsx';
import {ArrowRight} from 'lucide-react';
import SiteLink from '@site/src/components/SiteLink';

type LinkCardProps = {
  actionLabel?: string;
  children: React.ReactNode;
  className?: string;
  docId?: string;
  eyebrow?: string;
  title: string;
  to?: string;
};

export default function LinkCard({
  actionLabel = 'Open',
  children,
  className,
  docId,
  eyebrow,
  title,
  to,
}: LinkCardProps): React.JSX.Element {
  return (
    <SiteLink className={clsx('linkCard', className)} docId={docId} to={to}>
      {eyebrow ? <span className="linkCard__eyebrow">{eyebrow}</span> : null}
      <span className="linkCard__title">
        {title}
      </span>
      <span className="linkCard__body">{children}</span>
      <span className="linkCard__action">
        {actionLabel}
        <ArrowRight size={16} />
      </span>
    </SiteLink>
  );
}
