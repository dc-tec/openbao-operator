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
      {title ? <figcaption className="diagramFrame__title">{title}</figcaption> : null}
      <div className="diagramFrame__canvas">
        <Mermaid value={code} />
      </div>
      {caption ? <p className="diagramFrame__caption">{caption}</p> : null}
    </figure>
  );
}
