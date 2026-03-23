import React from 'react';
import clsx from 'clsx';

type CardGridProps = {
  children: React.ReactNode;
  className?: string;
};

export default function CardGrid({
  children,
  className,
}: CardGridProps): React.JSX.Element {
  return <div className={clsx('cardGrid', className)}>{children}</div>;
}
