import React, {useMemo} from 'react';
import Link from '@docusaurus/Link';
import {useDocsVersionCandidates} from '@docusaurus/plugin-content-docs/client';

type SiteLinkProps = Omit<React.ComponentProps<typeof Link>, 'to'> & {
  docId?: string;
  to?: string;
};

export default function SiteLink({
  docId,
  to,
  ...props
}: SiteLinkProps): React.JSX.Element {
  const versions = useDocsVersionCandidates();
  const resolvedTo = useMemo(() => {
    if (!docId) {
      return to ?? '#';
    }

    const match = versions
      .flatMap((version) => version.docs)
      .find((doc) => doc.id === docId && doc.path);

    return match?.path ?? to ?? '#';
  }, [docId, to, versions]);

  return <Link to={resolvedTo} {...props} />;
}
