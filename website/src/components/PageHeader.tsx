import React from 'react';
import clsx from 'clsx';
import renderInlineCode from '@site/src/components/renderInlineCode';

type PageHeaderProps = {
  title: React.ReactNode;
  lede?: React.ReactNode;
  className?: string;
};

export default function PageHeader({
  title,
  lede,
  className,
}: PageHeaderProps): React.JSX.Element {
  return (
    <header className={clsx('pageHeader', className)}>
      <h1>{renderInlineCode(title)}</h1>
      {lede ? <p className="pageHeader__lede">{renderInlineCode(lede)}</p> : null}
    </header>
  );
}
