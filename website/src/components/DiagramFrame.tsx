import React from 'react';
import Mermaid from '@theme/Mermaid';

type DiagramFrameProps = {
  caption?: React.ReactNode;
  code: string;
  title?: string;
};

export default function DiagramFrame({
  caption,
  code,
  title,
}: DiagramFrameProps): React.JSX.Element {
  return (
    <figure className="diagramFrame">
      {title || caption ? (
        <figcaption className="diagramFrame__header">
          <p className="diagramFrame__eyebrow">Diagram</p>
          {title ? <p className="diagramFrame__title">{title}</p> : null}
          {caption ? <p className="diagramFrame__caption">{caption}</p> : null}
        </figcaption>
      ) : null}
      <div className="diagramFrame__shell">
        <div className="diagramFrame__canvas">
          <Mermaid value={code} />
        </div>
      </div>
    </figure>
  );
}
