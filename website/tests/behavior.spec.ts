import {expect, test} from '@playwright/test';

test('local search is available and can find a known docs page', async ({page}) => {
  await page.goto('docs/next/get-started');

  const searchBox = page.getByRole('combobox', {name: /Search Search\.\.\./});
  await expect(searchBox).toBeVisible();

  await searchBox.click();
  await page.keyboard.type('compatibility');
  await expect(page.locator('.aa-Panel')).toBeVisible();

  const compatibilityResult = page.locator('a[href$="/openbao-operator/docs/reference/compatibility"]').first();
  await expect(compatibilityResult).toBeVisible();
});

test('docs navbar dropdown routes to validated deployments', async ({page}) => {
  await page.goto('');

  const docsDropdown = page.locator('.navbar').getByText('Docs', {exact: true}).first();
  await docsDropdown.hover();

  const validatedLink = page.locator('.dropdown__menu').getByRole('link', {
    name: 'Validated Deployments',
    exact: true,
  });
  await expect(validatedLink).toBeVisible();
  await validatedLink.click();

  await expect(page).toHaveURL(/\/openbao-operator\/docs\/validated-deployments$/);
  await expect(
    page.getByRole('heading', {
      name: 'Validated deployment baselines',
    }),
  ).toBeVisible();
});

test('version dropdown switches from next docs to the stable release line', async ({page}) => {
  await page.goto('docs/next/get-started/deployment-decision-guide');

  const versionDropdown = page.locator('.navbar').getByText(/^next$/, {exact: true}).first();
  await versionDropdown.hover();

  const archivedRelease = page.locator('.dropdown__menu').getByRole('link', {
    name: '0.5.x',
    exact: true,
  });
  await expect(archivedRelease).toBeVisible();
  await archivedRelease.click();

  await expect(page).toHaveURL(/\/openbao-operator\/docs\/get-started\/deployment-decision-guide$/);
  await expect(page.getByText('Published release documentation')).toBeVisible();
  await expect(page.getByText('Version: 0.5.x')).toBeVisible();
});

test.describe('curated legacy redirects stay alive', () => {
  const redirects = [
    ['latest/security', /\/openbao-operator\/docs\/security$/],
    ['latest/architecture', /\/openbao-operator\/docs\/architecture$/],
    ['latest/reference/compatibility', /\/openbao-operator\/docs\/reference\/compatibility$/],
    ['latest/contributing', /\/openbao-operator\/contribute$/],
    ['dev/security', /\/openbao-operator\/docs\/next\/security$/],
  ] as const;

  for (const [sourcePath, targetPattern] of redirects) {
    test(`${sourcePath} redirects into the intended destination`, async ({page}) => {
      await page.goto(sourcePath);
      await expect(page).toHaveURL(targetPattern);
    });
  }
});

test('feedback buttons acknowledge locally and route detailed feedback into the docs template', async ({page}) => {
  await page.goto('docs/next/get-started/deployment-decision-guide');

  await page.getByRole('button', {name: 'Yes'}).click();
  await expect(page.getByText('Thanks. Keep using the issue link when a page needs work.')).toBeVisible();

  const needsWork = page.getByRole('link', {name: 'Needs work'});
  await expect(needsWork).toHaveAttribute(
    'href',
    /github\.com\/dc-tec\/openbao-operator\/issues\/new\?template=docs_feedback\.yml&labels=documentation&page_path=/,
  );
  await expect(needsWork).toHaveAttribute(
    'href',
    /page_url=http%3A%2F%2F127\.0\.0\.1%3A4173%2Fopenbao-operator%2Fdocs%2Fnext%2Fget-started%2Fdeployment-decision-guide/,
  );
});

test('footer edge manifests link points to the direct install manifest', async ({page}) => {
  await page.goto('');

  await expect(page.getByRole('link', {name: 'Edge Manifests'})).toHaveAttribute(
    'href',
    'https://dc-tec.github.io/openbao-operator/edge/latest/install.yaml',
  );
});

test('desktop docs sidebar can collapse and expand again', async ({page}) => {
  await page.goto('docs/next/get-started');

  const collapseButton = page.getByRole('button', {name: 'Collapse sidebar', exact: true});
  await expect(collapseButton).toBeVisible();
  await collapseButton.click();

  const expandButton = page.getByRole('button', {name: 'Expand sidebar', exact: true});
  await expect(expandButton).toBeVisible();
  await expandButton.click();

  await expect(page.getByRole('button', {name: 'Collapse sidebar', exact: true})).toBeVisible();
});
