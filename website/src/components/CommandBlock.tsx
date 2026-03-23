import React from 'react';
import clsx from 'clsx';
import CodeBlock from '@theme/CodeBlock';

type CommandBlockProps = {
  children?: React.ReactNode;
  code: string;
  label?: 'apply' | 'configure' | 'inspect' | 'output' | 'verify';
  language: string;
  title?: string;
};

const labelText: Record<NonNullable<CommandBlockProps['label']>, string> = {
  apply: 'Apply',
  configure: 'Configure',
  inspect: 'Inspect',
  output: 'Output',
  verify: 'Verify',
};

export default function CommandBlock({
  children,
  code,
  label = 'apply',
  language,
  title,
}: CommandBlockProps): React.JSX.Element {
  return (
    <section className={clsx('commandBlock', `commandBlock--${label}`)}>
      {label || title ? (
        <header className="commandBlock__header">
          <div className="commandBlock__headerCopy">
            <p className="commandBlock__eyebrow">{labelText[label]}</p>
            {title ? <p className="commandBlock__title">{title}</p> : null}
          </div>
          <p className="commandBlock__meta">{language}</p>
        </header>
      ) : null}
      <div className="commandBlock__code">
        <CodeBlock language={language}>{code}</CodeBlock>
      </div>
      {children ? <div className="commandBlock__body">{children}</div> : null}
    </section>
  );
}
