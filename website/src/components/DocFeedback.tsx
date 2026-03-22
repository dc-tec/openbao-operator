import React from 'react';
import {ThumbsDown, ThumbsUp} from 'lucide-react';

declare global {
  interface Window {
    gtag?: (
      command: string,
      action: string,
      params?: Record<string, string | number | boolean>,
    ) => void;
  }
}

function emitFeedback(helpful: boolean): void {
  if (typeof window === 'undefined') {
    return;
  }

  window.gtag?.('event', 'docs_feedback', {
    event_category: 'docs',
    event_label: window.location.pathname,
    helpful,
  });
}

function issueUrl(): string {
  if (typeof window === 'undefined') {
    return 'https://github.com/dc-tec/openbao-operator/issues/new';
  }

  const page = window.location.href;
  const title = encodeURIComponent(`docs: feedback for ${window.location.pathname}`);
  const body = encodeURIComponent(
    [
      '## What page needs work?',
      page,
      '',
      '## What was confusing, missing, or incorrect?',
      '',
      '## What were you trying to do?',
      '',
      '## Suggested improvement',
      '',
    ].join('\n'),
  );

  return `https://github.com/dc-tec/openbao-operator/issues/new?labels=documentation&title=${title}&body=${body}`;
}

export default function DocFeedback(): React.JSX.Element {
  return (
    <section className="docFeedback" aria-label="Documentation feedback">
      <div className="docFeedback__header">
        <div>
          <p className="docFeedback__title">Was this page helpful?</p>
          <p className="docFeedback__subtitle">
            Feedback events are tracked in GA4. Detailed suggestions route into
            GitHub where the maintainers already work.
          </p>
        </div>
        <div className="docFeedback__actions">
          <button
            className="button button--secondary button--sm"
            type="button"
            onClick={() => emitFeedback(true)}
          >
            <ThumbsUp size={16} />
            Yes
          </button>
          <a
            className="button button--outline button--sm"
            href={issueUrl()}
            onClick={() => emitFeedback(false)}
            rel="noreferrer"
            target="_blank"
          >
            <ThumbsDown size={16} />
            Needs work
          </a>
        </div>
      </div>
    </section>
  );
}
