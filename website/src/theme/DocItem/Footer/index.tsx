import React from 'react';
import OriginalDocItemFooter from '@theme-original/DocItem/Footer';
import DocFeedback from '@site/src/components/DocFeedback';
import DocVersionBanner from '@site/src/components/DocVersionBanner';

export default function DocItemFooter(): React.JSX.Element {
  return (
    <>
      <DocVersionBanner />
      <OriginalDocItemFooter />
      <DocFeedback />
    </>
  );
}
