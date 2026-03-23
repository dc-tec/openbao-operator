import React from 'react';

function renderInlineCodeString(value: string): React.ReactNode {
  const pattern = /`([^`]+)`/g;
  const parts: React.ReactNode[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  let codeIndex = 0;

  while ((match = pattern.exec(value)) !== null) {
    if (match.index > lastIndex) {
      parts.push(value.slice(lastIndex, match.index));
    }

    parts.push(<code key={`inline-code-${codeIndex}`}>{match[1]}</code>);
    codeIndex += 1;
    lastIndex = match.index + match[0].length;
  }

  if (parts.length === 0) {
    return value;
  }

  if (lastIndex < value.length) {
    parts.push(value.slice(lastIndex));
  }

  return parts;
}

export default function renderInlineCode(node: React.ReactNode): React.ReactNode {
  if (typeof node === 'string') {
    return renderInlineCodeString(node);
  }

  if (Array.isArray(node)) {
    return node.map((child, index) => (
      <React.Fragment key={`inline-node-${index}`}>
        {renderInlineCode(child)}
      </React.Fragment>
    ));
  }

  if (React.isValidElement<{children?: React.ReactNode}>(node) && node.props.children) {
    return React.cloneElement(
      node,
      undefined,
      React.Children.map(node.props.children, (child) => renderInlineCode(child)),
    );
  }

  return node;
}
