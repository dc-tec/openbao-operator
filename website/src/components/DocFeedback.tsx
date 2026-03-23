import React, {useEffect, useState} from 'react';
import {ThumbsDown, ThumbsUp} from 'lucide-react';

function issueUrl(): string {
  if (typeof window === 'undefined') {
    return 'https://github.com/dc-tec/openbao-operator/issues/new';
  }

  const page = window.location.href;
  const pagePath = encodeURIComponent(window.location.pathname);
  const pageUrl = encodeURIComponent(page);

  return `https://github.com/dc-tec/openbao-operator/issues/new?template=docs_feedback.yml&labels=documentation&page_path=${pagePath}&page_url=${pageUrl}`;
}

export default function DocFeedback(): React.JSX.Element {
  const [helpfulAcknowledged, setHelpfulAcknowledged] = useState(false);
  const [issueHref, setIssueHref] = useState('https://github.com/dc-tec/openbao-operator/issues/new');

  useEffect(() => {
    setIssueHref(issueUrl());
  }, []);

  return (
    <section className="docFeedback" aria-label="Documentation feedback">
      <div className="docFeedback__header">
        <div>
          <p className="docFeedback__title">Was this page helpful?</p>
          <p className="docFeedback__subtitle">
            Use <strong>Needs work</strong> to open a structured GitHub issue for this page.
            The <strong>Yes</strong> button only acknowledges the signal locally.
          </p>
          {helpfulAcknowledged ? (
            <p className="docFeedback__ack" role="status">
              Thanks. Keep using the issue link when a page needs work.
            </p>
          ) : null}
        </div>
        <div className="docFeedback__actions">
          <button
            className="button button--secondary button--sm"
            type="button"
            onClick={() => setHelpfulAcknowledged(true)}
          >
            <ThumbsUp size={16} />
            Yes
          </button>
          <a
            className="button button--outline button--sm"
            href={issueHref}
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
