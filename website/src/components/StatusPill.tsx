import React from 'react';

type StatusPillProps = {
  children: React.ReactNode;
};

export default function StatusPill({children}: StatusPillProps): React.JSX.Element {
  return <span className="statusPill">{children}</span>;
}
