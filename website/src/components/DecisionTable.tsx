import React from 'react';
import clsx from 'clsx';

type DecisionRow = {
  cells: React.ReactNode[];
  emphasis?: 'neutral' | 'recommended' | 'caution';
};

type DecisionTableProps = {
  caption?: string;
  columns: React.ReactNode[];
  kind?: 'decision' | 'reference';
  rows: DecisionRow[];
  title?: string;
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
      {title ? <p className="decisionTable__title">{title}</p> : null}
      <div className="decisionTable__scroll">
        <table>
          {caption ? <caption>{caption}</caption> : null}
          <thead>
            <tr>
              {columns.map((column, index) => (
                <th key={`column-${index}`} scope="col">
                  {column}
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
                  <td key={`cell-${rowIndex}-${cellIndex}`}>{cell}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
