import React from 'react';
import Link from '@docusaurus/Link';
import {useDocsVersion} from '@docusaurus/plugin-content-docs/client';

export default function DocVersionBanner(): React.JSX.Element | null {
  const version = useDocsVersion();
  const versionName = version?.version;

  if (!versionName) {
    return null;
  }

  if (versionName === 'current') {
    return (
      <div className="docBanner">
        <strong>Next release documentation</strong>
        <p>
          You are reading the unreleased <code>main</code> docs. Use the version
          menu for the newest published release, or check the{' '}
          <Link to="/releases">release notes</Link> for what is already out.
        </p>
      </div>
    );
  }

  if (versionName.includes('-')) {
    return (
      <div className="docBanner">
        <strong>Prerelease documentation</strong>
        <p>
          This version tracks a prerelease build. Features and behavior may
          change before the next stable release.
        </p>
      </div>
    );
  }

  return (
    <div className="docBanner">
      <strong>Published release documentation</strong>
      <p>
        You are reading docs for version <code>{version.label}</code>. Use the
        version menu to switch to <code>next</code> or another archived release.
      </p>
    </div>
  );
}
