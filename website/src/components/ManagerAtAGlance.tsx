import React from 'react';
import renderInlineCode from '@site/src/components/renderInlineCode';

type ManagerSummarySection = {
  items: React.ReactNode[];
  label: string;
};

type ManagerAtAGlanceProps = {
  sections: ManagerSummarySection[];
  title?: React.ReactNode;
};

export default function ManagerAtAGlance({
  sections,
  title = 'At a glance',
}: ManagerAtAGlanceProps): React.JSX.Element {
  return (
    <section className="managerAtAGlance">
      <p className="managerAtAGlance__title">{renderInlineCode(title)}</p>
      <div className="managerAtAGlance__grid">
        {sections.map((section, sectionIndex) => (
          <section key={`${section.label}-${sectionIndex}`} className="managerAtAGlance__section">
            <p className="managerAtAGlance__label">{renderInlineCode(section.label)}</p>
            <ul className="managerAtAGlance__list">
              {section.items.map((item, index) => (
                <li key={`${section.label}-${index}`} className="managerAtAGlance__item">
                  {renderInlineCode(item)}
                </li>
              ))}
            </ul>
          </section>
        ))}
      </div>
    </section>
  );
}
