import React from 'react';
import clsx from 'clsx';
import renderInlineCode from '@site/src/components/renderInlineCode';

type DecisionRow = {
  cells: React.ReactNode[];
  emphasis?: 'neutral' | 'recommended' | 'caution';
};

type DecisionTableProps = {
  caption?: React.ReactNode;
  columns: React.ReactNode[];
  kind?: 'decision' | 'reference';
  rows: DecisionRow[];
  title?: React.ReactNode;
};

const kindLabels: Record<NonNullable<DecisionTableProps['kind']>, string> = {
  decision: 'Decision matrix',
  reference: 'Reference table',
};

export default function DecisionTable({
  caption,
  columns,
  kind = 'decision',
  rows,
  title,
}: DecisionTableProps): React.JSX.Element {
  return (
    <section className={clsx('decisionTable', `decisionTable--${kind}`)}>
      {title || caption ? (
        <header className="decisionTable__header">
          <p className="decisionTable__eyebrow">{kindLabels[kind]}</p>
          {title ? <p className="decisionTable__title">{renderInlineCode(title)}</p> : null}
          {caption ? <p className="decisionTable__caption">{renderInlineCode(caption)}</p> : null}
        </header>
      ) : null}
      <div className="decisionTable__scroll">
        <table>
          {title || caption ? (
            <caption className="decisionTable__srOnly">
              {title ? <>{renderInlineCode(title)}. </> : null}
              {renderInlineCode(caption)}
            </caption>
          ) : null}
          <thead>
            <tr>
              {columns.map((column, index) => (
                <th key={`column-${index}`} scope="col">
                  {renderInlineCode(column)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, rowIndex) => (
              <tr
                key={`row-${rowIndex}`}
                className={clsx(row.emphasis && `decisionTable__row--${row.emphasis}`)}
              >
                {row.cells.map((cell, cellIndex) => (
                  <td key={`cell-${rowIndex}-${cellIndex}`}>{renderInlineCode(cell)}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
